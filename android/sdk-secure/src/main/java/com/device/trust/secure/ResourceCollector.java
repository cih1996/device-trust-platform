package com.device.trust.secure;

import android.app.ActivityManager;
import android.content.Context;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ApplicationInfo;
import android.content.pm.Signature;
import android.os.Build;
import android.os.StatFs;
import android.os.Environment;
import android.os.SystemClock;
import android.os.BatteryManager;
import android.util.DisplayMetrics;
import android.content.Intent;
import android.content.IntentFilter;
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
        put(signals,"integrityNative",new JSONObject(NativeFrame.integrity()));
        put(signals,"osInfo",new JSONObject().put("release",Build.VERSION.RELEASE).put("sdk",Build.VERSION.SDK_INT).put("patch",Build.VERSION.SECURITY_PATCH).put("buildId",Build.ID).put("display",Build.DISPLAY).put("type",Build.TYPE).put("tags",Build.TAGS).put("bootloader",Build.BOOTLOADER).put("uptimeMillis",SystemClock.elapsedRealtime()));
        put(signals,"memory",new JSONObject().put("totalBytes",memory.totalMem).put("availableBytes",memory.availMem).put("lowMemory",memory.lowMemory));
        StatFs storage=new StatFs(Environment.getDataDirectory().getPath());put(signals,"storage",new JSONObject().put("totalBytes",storage.getTotalBytes()).put("availableBytes",storage.getAvailableBytes()).put("scope","data_partition"));
        DisplayMetrics display=context.getResources().getDisplayMetrics();put(signals,"display",new JSONObject().put("widthPixels",display.widthPixels).put("heightPixels",display.heightPixels).put("densityDpi",display.densityDpi).put("density",display.density).put("fontScale",context.getResources().getConfiguration().fontScale));
        Intent battery=context.registerReceiver(null,new IntentFilter(Intent.ACTION_BATTERY_CHANGED));
        if(battery!=null)put(signals,"battery",new JSONObject().put("level",battery.getIntExtra(BatteryManager.EXTRA_LEVEL,-1)).put("scale",battery.getIntExtra(BatteryManager.EXTRA_SCALE,-1)).put("status",battery.getIntExtra(BatteryManager.EXTRA_STATUS,-1)).put("plugged",battery.getIntExtra(BatteryManager.EXTRA_PLUGGED,-1)).put("temperatureTenthsC",battery.getIntExtra(BatteryManager.EXTRA_TEMPERATURE,-1)).put("voltageMillivolts",battery.getIntExtra(BatteryManager.EXTRA_VOLTAGE,-1)));
        else signals.put("battery",new JSONObject().put("status","unavailable").put("reason","system returned no battery snapshot"));
        JSONArray sensors=new JSONArray();SensorManager manager=(SensorManager)context.getSystemService(Context.SENSOR_SERVICE);
        for(Sensor s:manager.getSensorList(Sensor.TYPE_ALL))sensors.put(new JSONObject().put("type",s.getType()).put("vendor",s.getVendor()).put("name",s.getName()).put("version",s.getVersion()).put("resolution",s.getResolution()).put("maxRange",s.getMaximumRange()));
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
        @SuppressWarnings("deprecation") PackageInfo own=pm.getPackageInfo(context.getPackageName(),Build.VERSION.SDK_INT>=28?PackageManager.GET_SIGNING_CERTIFICATES:PackageManager.GET_SIGNATURES);
        @SuppressWarnings("deprecation") Signature[] ownSignatures=Build.VERSION.SDK_INT>=28?own.signingInfo.getApkContentsSigners():own.signatures;
        JSONArray ownDigests=new JSONArray();for(Signature signature:ownSignatures)ownDigests.put(hex(MessageDigest.getInstance("SHA-256").digest(signature.toByteArray())));
        @SuppressWarnings("deprecation") String installer=Build.VERSION.SDK_INT>=30?pm.getInstallSourceInfo(context.getPackageName()).getInstallingPackageName():pm.getInstallerPackageName(context.getPackageName());
        put(signals,"application",new JSONObject().put("package",own.packageName).put("versionName",own.versionName).put("versionCode",Build.VERSION.SDK_INT>=28?own.getLongVersionCode():own.versionCode).put("signers",ownDigests).put("installer",installer==null?JSONObject.NULL:installer));
    }
    private static void put(JSONObject signals,String name,Object value)throws Exception{signals.put(name,new JSONObject().put("status","available").put("value",value));}
    private static String hex(byte[] data){char[] out=new char[data.length*2];char[] digits="0123456789abcdef".toCharArray();for(int i=0;i<data.length;i++){out[i*2]=digits[(data[i]&255)>>>4];out[i*2+1]=digits[data[i]&15];}return new String(out);}
}
