package com.device.trust.secure;

import android.app.ActivityManager;
import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ApplicationInfo;
import android.content.pm.Signature;
import android.os.Build;
import android.hardware.Sensor;
import android.hardware.SensorManager;
import org.json.JSONObject;
import org.json.JSONArray;
import java.security.MessageDigest;

/** Targeted system-resource observations; no installed-app inventory or identifiers. */
public final class ResourceCollector {
    private ResourceCollector(){}
    public static final String[] PACKAGES={"android","com.android.settings","com.android.systemui","com.miui.securitycenter","com.miui.home","com.xiaomi.market","com.huawei.hwid","com.huawei.appmarket","com.heytap.market","com.oplus.safecenter","com.bbk.appstore","com.bbk.launcher2","com.hihonor.appmarket"};
    public static void collect(Context context,JSONObject report,boolean consent) throws Exception {
        if(!consent)throw new IllegalStateException("Consent required before collecting resources");
        JSONObject signals=report.getJSONObject("signals");
        ActivityManager.MemoryInfo memory=new ActivityManager.MemoryInfo();
        ((ActivityManager)context.getSystemService(Context.ACTIVITY_SERVICE)).getMemoryInfo(memory);
        JSONObject java=new JSONObject().put("brand",Build.BRAND).put("manufacturer",Build.MANUFACTURER).put("model",Build.MODEL).put("device",Build.DEVICE).put("hardware",Build.HARDWARE).put("board",Build.BOARD).put("product",Build.PRODUCT).put("supportedAbis",new JSONArray(Build.SUPPORTED_ABIS)).put("memoryBytes",memory.totalMem).put("availableCores",Runtime.getRuntime().availableProcessors());
        if(Build.VERSION.SDK_INT>=31)java.put("socModel",Build.SOC_MODEL).put("socManufacturer",Build.SOC_MANUFACTURER);
        put(signals,"resourceJava",java);put(signals,"resourceNative",new JSONObject(NativeFrame.resources()));
        JSONArray sensors=new JSONArray();SensorManager manager=(SensorManager)context.getSystemService(Context.SENSOR_SERVICE);
        for(int type:new int[]{Sensor.TYPE_ACCELEROMETER,Sensor.TYPE_GYROSCOPE,Sensor.TYPE_MAGNETIC_FIELD,Sensor.TYPE_PRESSURE}){Sensor s=manager.getDefaultSensor(type);if(s!=null)sensors.put(new JSONObject().put("type",type).put("vendor",s.getVendor()).put("name",s.getName()).put("version",s.getVersion()));}
        put(signals,"sensors",sensors);
        JSONArray packages=new JSONArray();PackageManager pm=context.getPackageManager();
        for(String name:PACKAGES){JSONObject item=new JSONObject().put("package",name);
            try {
                @SuppressWarnings("deprecation") PackageInfo info=pm.getPackageInfo(name,Build.VERSION.SDK_INT>=28?PackageManager.GET_SIGNING_CERTIFICATES:PackageManager.GET_SIGNATURES);
                @SuppressWarnings("deprecation") Signature[] signatures=Build.VERSION.SDK_INT>=28?info.signingInfo.getApkContentsSigners():info.signatures;
                JSONArray digests=new JSONArray();for(Signature signature:signatures)digests.put(hex(MessageDigest.getInstance("SHA-256").digest(signature.toByteArray())));
                item.put("status","visible").put("signers",digests).put("system",(info.applicationInfo.flags&ApplicationInfo.FLAG_SYSTEM)!=0).put("version",Build.VERSION.SDK_INT>=28?info.getLongVersionCode():info.versionCode);
            } catch(PackageManager.NameNotFoundException missing){item.put("status","not_visible_or_absent");}
            catch(SecurityException denied){item.put("status","permission_denied").put("error",denied.toString());}
            packages.put(item);
        }
        put(signals,"systemPackages",packages);
    }
    private static void put(JSONObject signals,String name,Object value)throws Exception{signals.put(name,new JSONObject().put("status","available").put("value",value));}
    private static String hex(byte[] data){char[] out=new char[data.length*2];char[] digits="0123456789abcdef".toCharArray();for(int i=0;i<data.length;i++){out[i*2]=digits[(data[i]&255)>>>4];out[i*2+1]=digits[data[i]&15];}return new String(out);}
}
