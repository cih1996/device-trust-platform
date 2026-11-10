package com.device.trust.secure;

import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLPeerUnverifiedException;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.TrustManagerFactory;
import javax.net.ssl.X509TrustManager;
import android.net.http.X509TrustManagerExtensions;
import java.net.URL;
import java.security.MessageDigest;
import java.security.cert.Certificate;
import java.security.cert.X509Certificate;
import java.security.cert.CertificateException;
import java.security.KeyStore;
import java.io.*;
import java.nio.charset.StandardCharsets;
import android.util.Base64;
import org.json.JSONObject;

public final class PinnedHttp {
    private PinnedHttp() {}
    public static JSONObject post(String endpoint,String[] pins,String bearer,String contentType,byte[] body) throws Exception {
        URL url=new URL(endpoint);
        if(!"https".equals(url.getProtocol()))throw new IllegalArgumentException("HTTPS required");
        if(pins==null||pins.length==0)throw new IllegalArgumentException("SPKI certificate pins required");
        HttpsURLConnection connection=(HttpsURLConnection)url.openConnection();
        try {
            byte[][] expectedPins=new byte[pins.length][];
            for(int i=0;i<pins.length;i++){expectedPins[i]=Base64.decode(pins[i],Base64.DEFAULT);if(expectedPins[i].length!=32)throw new IllegalArgumentException("SHA-256 SPKI pin required");}
            TrustManagerFactory factory=TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());factory.init((KeyStore)null);
            X509TrustManager platform=null;for(TrustManager manager:factory.getTrustManagers())if(manager instanceof X509TrustManager)platform=(X509TrustManager)manager;
            if(platform==null)throw new java.security.GeneralSecurityException("platform trust manager unavailable");
            X509TrustManager original=platform;X509TrustManagerExtensions trusted=new X509TrustManagerExtensions(original);
            X509TrustManager pinned=new X509TrustManager(){
                public X509Certificate[] getAcceptedIssuers(){return original.getAcceptedIssuers();}
                public void checkClientTrusted(X509Certificate[] chain,String authType) throws CertificateException{original.checkClientTrusted(chain,authType);}
                public void checkServerTrusted(X509Certificate[] chain,String authType) throws CertificateException{
                    java.util.List<X509Certificate> validated=trusted.checkServerTrusted(chain,authType,url.getHost());
                    try {
                        for(X509Certificate cert:validated){byte[] digest=MessageDigest.getInstance("SHA-256").digest(cert.getPublicKey().getEncoded());for(byte[] expected:expectedPins)if(NativeFrame.pinMatches(digest,expected))return;}
                    } catch(java.security.GeneralSecurityException error){throw new CertificateException(error);}
                    throw new CertificateException("TLS SPKI pin mismatch");
                }
            };
            SSLContext tls=SSLContext.getInstance("TLS");tls.init(null,new TrustManager[]{pinned},null);connection.setSSLSocketFactory(tls.getSocketFactory());
            connection.setConnectTimeout(10000);connection.setReadTimeout(15000);connection.setInstanceFollowRedirects(false);
            connection.setRequestMethod("POST");connection.setRequestProperty("Content-Type",contentType);
            if(bearer!=null&&!bearer.isEmpty())connection.setRequestProperty("Authorization","Bearer "+bearer);
            connection.setDoOutput(true);connection.setFixedLengthStreamingMode(body.length);
            connection.connect();
            try(OutputStream output=connection.getOutputStream()){output.write(body);}
            int code=connection.getResponseCode();InputStream input=code>=200&&code<300?connection.getInputStream():connection.getErrorStream();
            ByteArrayOutputStream output=new ByteArrayOutputStream();
            if(input!=null)try(InputStream stream=input){byte[] buffer=new byte[4096];int count;while((count=stream.read(buffer))!=-1){output.write(buffer,0,count);if(output.size()>512*1024)throw new IOException("response exceeds 512 KiB");}}
            String text=new String(output.toByteArray(),StandardCharsets.UTF_8);
            if(code<200||code>=300)throw new IOException("HTTP "+code+": "+text);
            return new JSONObject(text);
        } finally {connection.disconnect();}
    }
}
