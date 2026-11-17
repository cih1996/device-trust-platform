package com.device.trust.secure;

import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import android.os.Looper;
import android.content.Context;
import android.content.SharedPreferences;
import org.json.JSONObject;
import org.json.JSONArray;
import java.security.*;
import java.security.cert.Certificate;
import java.security.spec.ECGenParameterSpec;
import java.nio.charset.StandardCharsets;

public final class SecureReporter {
    private SecureReporter() {}
    private static String alias(String appId) throws GeneralSecurityException {return "dtp-"+hex(MessageDigest.getInstance("SHA-256").digest(appId.getBytes(StandardCharsets.UTF_8)));}
    private static KeyStore store() throws Exception {KeyStore store=KeyStore.getInstance("AndroidKeyStore");store.load(null);return store;}
    public static String keyId(Context context,String appId) throws Exception {
        String active=context.getSharedPreferences("dtp_secure",Context.MODE_PRIVATE).getString(alias(appId),null);
        if(active==null)return "";
        Certificate cert=store().getCertificate(active);
        if(cert==null)throw new GeneralSecurityException("enrolled device key was removed");
        return hex(MessageDigest.getInstance("SHA-256").digest(cert.getPublicKey().getEncoded()));
    }
    public static void reset(Context context,String appId) throws Exception {
        SharedPreferences prefs=context.getSharedPreferences("dtp_secure",Context.MODE_PRIVATE);String slot=alias(appId),active=prefs.getString(slot,null);
        if(active!=null)store().deleteEntry(active);
        if(!prefs.edit().remove(slot).commit())throw new java.io.IOException("could not reset device enrollment");
    }
    public static JSONObject upload(Context context,String platform,String[] pins,JSONObject challenge,JSONObject report) throws Exception {
        if(Looper.myLooper()==Looper.getMainLooper())throw new IllegalStateException("Secure upload must run off the main thread");
        String appId=challenge.getString("appId");
        if(!appId.equals(report.getString("appId")))throw new GeneralSecurityException("report application mismatch");
        String slot=alias(appId);SharedPreferences prefs=context.getSharedPreferences("dtp_secure",Context.MODE_PRIVATE);KeyStore store=store();
        byte[] nonce=Base64.decode(challenge.getString("nonce"),Base64.DEFAULT);
        String expected=challenge.getString("keyId");
        String alias=expected.isEmpty()?slot+"-"+challenge.getString("id"):prefs.getString(slot,null);
        String attestationError=null;
        if(expected.isEmpty()) {
            if(!store.containsAlias(alias)) {
                // Attested by default: the server verifies package, signer and boot state from the chain.
                try {generateKey(alias,nonce);}
                catch(ProviderException e) {
                    // Some Keystores cannot attest (e.g. broken factory keybox). When the app
                    // does not require hardware, enroll an unattested key and report why.
                    if(challenge.getBoolean("hardwareRequired"))throw e;
                    if(store.containsAlias(alias))store.deleteEntry(alias);
                    attestationError=describe(e);
                    generateKey(alias,null);
                }
            }
        } else if(!expected.equals(keyId(context,appId)))throw new GeneralSecurityException("local device key differs from server challenge");
        PrivateKey privateKey=(PrivateKey)store.getKey(alias,null);
        if(privateKey==null)throw new GeneralSecurityException("device signing key unavailable");
        byte[] id=MessageDigest.getInstance("SHA-256").digest(store.getCertificate(alias).getPublicKey().getEncoded());byte[] challengeId=unhex(challenge.getString("id"));
        byte[] server=Base64.decode(challenge.getString("serverPublicKey"),Base64.DEFAULT);
        if(expected.isEmpty()) {
            JSONArray chain=new JSONArray();Certificate[] certs=store.getCertificateChain(alias);
            if(certs==null)throw new GeneralSecurityException("device certificate chain unavailable");
            for(Certificate cert:certs)chain.put(Base64.encodeToString(cert.getEncoded(),Base64.NO_WRAP));
            JSONObject body=new JSONObject().put("certificateChain",chain);
            if(attestationError!=null)body.put("attestationError",attestationError);
            byte[] enrollment=body.toString().getBytes(StandardCharsets.UTF_8);
            byte[] frame=NativeFrame.protect(1,challengeId,id,nonce,server,enrollment,privateKey);
            PinnedHttp.post(platform+"/v1/secure",pins,null,"application/octet-stream",frame);
            if(!prefs.edit().putString(slot,alias).commit())throw new java.io.IOException("could not persist device enrollment");
        }
        LanProbe.attach(challenge,report);
        byte[] payload=report.toString().getBytes(StandardCharsets.UTF_8);
        byte[] frame=NativeFrame.protect(2,challengeId,id,nonce,server,payload,privateKey);
        JSONObject result=PinnedHttp.post(platform+"/v1/secure",pins,null,"application/octet-stream",frame);
        String deviceId=result.getString("deviceId");
        if(!prefs.edit().putString("deviceId:"+appId,deviceId).commit())throw new java.io.IOException("could not persist device ID");
        return result;
    }
    private static void generateKey(String alias,byte[] challenge) throws GeneralSecurityException {
        KeyPairGenerator generator=KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC,"AndroidKeyStore");
        KeyGenParameterSpec.Builder parameters=new KeyGenParameterSpec.Builder(alias,KeyProperties.PURPOSE_SIGN)
                .setAlgorithmParameterSpec(new ECGenParameterSpec("secp256r1"))
                .setDigests(KeyProperties.DIGEST_SHA256);
        if(challenge!=null)parameters.setAttestationChallenge(challenge);
        generator.initialize(parameters.build());
        generator.generateKeyPair();
    }
    private static String describe(Throwable e){StringBuilder out=new StringBuilder();for(Throwable t=e;t!=null&&out.length()<500;t=t.getCause()){if(out.length()>0)out.append(" <- ");out.append(t.getClass().getSimpleName()).append(": ").append(t.getMessage());}return out.length()>500?out.substring(0,500):out.toString();}
    public static String deviceId(Context context,String appId){return context.getSharedPreferences("dtp_secure",Context.MODE_PRIVATE).getString("deviceId:"+appId,"");}
    private static String hex(byte[] bytes){char[] out=new char[bytes.length*2];char[] digits="0123456789abcdef".toCharArray();for(int i=0;i<bytes.length;i++){out[2*i]=digits[(bytes[i]&255)>>>4];out[2*i+1]=digits[bytes[i]&15];}return new String(out);}
    private static byte[] unhex(String text){if((text.length()&1)!=0)throw new IllegalArgumentException("invalid hex length");byte[] out=new byte[text.length()/2];for(int i=0;i<out.length;i++){int a=Character.digit(text.charAt(2*i),16),b=Character.digit(text.charAt(2*i+1),16);if(a<0||b<0)throw new IllegalArgumentException("invalid hex");out[i]=(byte)((a<<4)|b);}return out;}
}
