import com.device.trust.secure.NativeFrame;
import java.security.*;
import java.security.spec.ECGenParameterSpec;
import java.security.interfaces.ECPrivateKey;
import java.nio.file.*;
import java.util.Base64;
import java.util.Arrays;

/** Host JNI interoperability probe; generates test keys only. */
public class NativeProtocolProbe {
    public static void main(String[] args) throws Exception {
        KeyPairGenerator generator=KeyPairGenerator.getInstance("EC");generator.initialize(new ECGenParameterSpec("secp256r1"));
        KeyPair signer=generator.generateKeyPair(),server=generator.generateKeyPair();
        byte[] challenge=new byte[24],salt=new byte[32];new SecureRandom().nextBytes(challenge);new SecureRandom().nextBytes(salt);
        byte[] keyId=MessageDigest.getInstance("SHA-256").digest(signer.getPublic().getEncoded());
        byte[] payload="{\"appId\":\"native-probe\",\"value\":17}".getBytes(java.nio.charset.StandardCharsets.UTF_8);
        byte[] frame=NativeFrame.protect(2,challenge,keyId,salt,server.getPublic().getEncoded(),payload,signer.getPrivate());
        if(!NativeFrame.pinMatches(keyId,keyId))throw new AssertionError("valid pin rejected");
        byte[] wrong=keyId.clone();wrong[0]^=1;if(NativeFrame.pinMatches(keyId,wrong))throw new AssertionError("wrong pin accepted");
        byte[] scalar=((ECPrivateKey)server.getPrivate()).getS().toByteArray();byte[] privateBytes=new byte[32];
        System.arraycopy(scalar,Math.max(0,scalar.length-32),privateBytes,Math.max(0,32-scalar.length),Math.min(32,scalar.length));
        Base64.Encoder b64=Base64.getEncoder();
        String json="{\"frame\":\""+b64.encodeToString(frame)+"\",\"serverPrivateKey\":\""+b64.encodeToString(privateBytes)+"\",\"nonce\":\""+b64.encodeToString(salt)+"\",\"publicKey\":\""+b64.encodeToString(signer.getPublic().getEncoded())+"\",\"payload\":\""+b64.encodeToString(payload)+"\"}";
        Files.writeString(Path.of(args[0]),json);
        try {NativeFrame.protect(2,new byte[1],keyId,salt,server.getPublic().getEncoded(),payload,signer.getPrivate());throw new AssertionError("invalid challenge accepted");}
        catch (java.security.GeneralSecurityException expected) {System.out.println("Original JNI validation error retained: "+expected);}
        System.out.println("Obfuscated JNI frame generated: "+frame.length+" bytes");
    }
}
