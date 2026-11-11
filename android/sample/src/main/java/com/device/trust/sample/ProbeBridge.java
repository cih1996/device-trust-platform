package com.device.trust.sample;

import android.content.Context;
import com.device.trust.DeviceTrust;
import com.device.trust.EnvironmentCollector;
import com.device.trust.RootBeerCollector;
import com.device.trust.secure.ResourceCollector;
import com.device.trust.secure.SecureReporter;
import com.device.trust.secure.PinnedHttp;
import com.device.trust.secure.DeviceTrustClient;
import org.json.JSONObject;
import java.io.File;
import java.io.FileInputStream;
import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;

/** Explicit instrumentation-only entry for the standalone sample, not an exported component. */
public final class ProbeBridge {
 public static String run(Context context)throws Exception{
  File file=new File(context.getExternalFilesDir(null),"dtp-test.json");byte[] data;
  try(FileInputStream input=new FileInputStream(file);ByteArrayOutputStream output=new ByteArrayOutputStream()){byte[] buffer=new byte[4096];int n;while((n=input.read(buffer))!=-1)output.write(buffer,0,n);data=output.toByteArray();}
  if(!file.delete())throw new java.io.IOException("could not remove private test configuration");
  JSONObject config=new JSONObject(new String(data,StandardCharsets.UTF_8));String base=config.getString("businessUrl");
  if(config.optBoolean("testProxy")){
   java.net.ProxySelector.setDefault(new java.net.ProxySelector(){
    public java.util.List<java.net.Proxy> select(java.net.URI uri){if(!"devicetrust-sdk-api.lingzidev.com".equals(uri.getHost()))throw new IllegalArgumentException("unexpected test destination");return java.util.Collections.singletonList(new java.net.Proxy(java.net.Proxy.Type.HTTP,new java.net.InetSocketAddress("127.0.0.1",19444)));}
    public void connectFailed(java.net.URI uri,java.net.SocketAddress address,java.io.IOException error){android.util.Log.e("DeviceTrustTest",error.toString(),error);}
   });
  }
  if(config.optBoolean("startupOnly")){
   DeviceTrustClient.Config options=new DeviceTrustClient.Config(BuildConfig.DEMO_APP_ID,base,BuildConfig.SERVER_PINS);
   boolean blocked=false;try{DeviceTrustClient.initialize(context,options,false,null);}catch(IllegalStateException expected){blocked=true;}if(!blocked)throw new IllegalStateException("collection before consent was accepted");
   final JSONObject[] captured={null};
   DeviceTrustClient.ExtraCollector extra=(ctx,snapshot)->{RootBeerCollector.collect(ctx,snapshot,true);EnvironmentCollector.collect(ctx,snapshot,true,EnvironmentCollector.Collection.WIFI,EnvironmentCollector.Collection.CELL,EnvironmentCollector.Collection.LOCATION);captured[0]=snapshot;};
   JSONObject first=DeviceTrustClient.initialize(context,options,true,extra);JSONObject second=DeviceTrustClient.initialize(context,options,true,extra);
   if(!first.getString("deviceId").equals(second.getString("deviceId")))throw new IllegalStateException("startup identity changed");
   return new JSONObject().put("consentBlocked",blocked).put("testProxy",config.optBoolean("testProxy")).put("deviceId",second.getString("deviceId")).put("keyId",SecureReporter.keyId(context,BuildConfig.DEMO_APP_ID)).put("reportId",second.getString("reportId")).put("state","registered_before_login").put("retryState","same_device_id").put("report",captured[0]).toString();
  }
  JSONObject login=post(base+"/login",null,new JSONObject().put("accountId",config.getString("account")).put("password",config.getString("password")));
  String appId=login.getString("appId"),session=login.getString("token");
  JSONObject prepared=post(base+"/prepare",session,new JSONObject().put("type","reward").put("keyId",SecureReporter.keyId(context,appId)));
  JSONObject report=DeviceTrust.collect(context,new DeviceTrust.Options(appId,false),true);
  ResourceCollector.collect(context,report,true);RootBeerCollector.collect(context,report,true);
  EnvironmentCollector.collect(context,report,true,EnvironmentCollector.Collection.WIFI,EnvironmentCollector.Collection.CELL,EnvironmentCollector.Collection.LOCATION);
  JSONObject sent=SecureReporter.upload(context,login.getString("platformUrl"),BuildConfig.SERVER_PINS,prepared.getJSONObject("challenge"),report);
  JSONObject completion=new JSONObject().put("eventId",prepared.getString("eventId")).put("reportId",sent.getString("reportId"));
  JSONObject outcome=post(base+"/complete",session,completion);
  JSONObject repeated=post(base+"/complete",session,completion);
  report.getJSONObject("signals").put("labHardwareProbe",new JSONObject().put("status","available").put("value",probeHardware()));
  return new JSONObject().put("testProxy",config.optBoolean("testProxy")).put("deviceId",sent.getString("deviceId")).put("keyId",SecureReporter.keyId(context,appId)).put("reportId",sent.getString("reportId")).put("eventId",prepared.getString("eventId")).put("state",outcome.getString("state")).put("retryState",repeated.getString("state")).put("report",report).toString();
 }
 private static JSONObject probeHardware()throws Exception{
  String alias="dtp-lab-hardware-probe";java.security.KeyStore store=java.security.KeyStore.getInstance("AndroidKeyStore");store.load(null);
  JSONObject result=new JSONObject();byte[] nonce=new byte[32];new java.security.SecureRandom().nextBytes(nonce);
  try{
   java.security.KeyPairGenerator generator=java.security.KeyPairGenerator.getInstance("EC","AndroidKeyStore");
   generator.initialize(new android.security.keystore.KeyGenParameterSpec.Builder(alias,android.security.keystore.KeyProperties.PURPOSE_SIGN).setAlgorithmParameterSpec(new java.security.spec.ECGenParameterSpec("secp256r1")).setDigests(android.security.keystore.KeyProperties.DIGEST_SHA256).setAttestationChallenge(nonce).build());generator.generateKeyPair();
   org.json.JSONArray chain=new org.json.JSONArray();for(java.security.cert.Certificate cert:store.getCertificateChain(alias))chain.put(android.util.Base64.encodeToString(cert.getEncoded(),android.util.Base64.NO_WRAP));
   result.put("status","generated").put("nonce",android.util.Base64.encodeToString(nonce,android.util.Base64.NO_WRAP)).put("chain",chain);
  }catch(Exception error){java.io.StringWriter text=new java.io.StringWriter();error.printStackTrace(new java.io.PrintWriter(text));result.put("status","error").put("error",text.toString());}
  finally{if(store.containsAlias(alias))store.deleteEntry(alias);}
  return result;
 }
 private static JSONObject post(String url,String token,JSONObject body)throws Exception{return PinnedHttp.post(url,BuildConfig.SERVER_PINS,token,"application/json",body.toString().getBytes(StandardCharsets.UTF_8));}
}
