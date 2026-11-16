package com.device.trust.secure;

import android.content.Context;
import android.os.Looper;
import com.device.trust.DeviceTrust;
import org.json.JSONObject;
import java.nio.charset.StandardCharsets;

/** Explicit app-start initialization. The host supplies consent and schedules the worker. */
public final class DeviceTrustClient {
 private DeviceTrustClient(){}
 public interface ExtraCollector {void collect(Context context,JSONObject report)throws Exception;}
 public static final class Config {
  public final String appId,businessUrl;public final String[] pins;
  public Config(String appId,String businessUrl,String[] pins){if(appId==null||appId.isEmpty())throw new IllegalArgumentException("public app ID required");this.appId=appId;this.businessUrl=businessUrl;this.pins=pins.clone();}
 }
 public static JSONObject initialize(Context context,Config config,boolean consent,ExtraCollector extra)throws Exception{
  if(!consent)throw new IllegalStateException("Consent required before device registration");
  if(Looper.myLooper()==Looper.getMainLooper())throw new IllegalStateException("Initialize on a background thread");
  // ANDROID_ID is scoped to the APK signer: the server links the same handset across apps you sign.
  JSONObject report=DeviceTrust.collect(context,new DeviceTrust.Options(config.appId,true),true);
  ResourceCollector.collect(context,report,true);if(extra!=null)extra.collect(context,report);
  JSONObject input=new JSONObject().put("appId",config.appId).put("installationId",report.getString("installationId")).put("keyId",SecureReporter.keyId(context,config.appId));
  JSONObject prepared=PinnedHttp.post(config.businessUrl+"/device/prepare",config.pins,null,"application/json",input.toString().getBytes(StandardCharsets.UTF_8));
  if(!config.appId.equals(prepared.getString("appId")))throw new java.security.GeneralSecurityException("bootstrap app mismatch");
  return SecureReporter.upload(context,prepared.getString("platformUrl"),config.pins,prepared.getJSONObject("challenge"),report);
 }
 public static String deviceId(Context context,String appId){return SecureReporter.deviceId(context,appId);}
}
