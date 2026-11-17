package com.device.trust.secure;

import android.accessibilityservice.AccessibilityServiceInfo;
import android.app.ActivityManager;
import android.content.Context;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.provider.Settings;
import android.view.accessibility.AccessibilityManager;
import org.json.JSONArray;
import org.json.JSONObject;

/** Risk tools, accessibility automation and Java-level hook traces. Targeted lookups only; no app inventory. */
final class RiskCollector {
    private RiskCollector(){}
    // Every name is declared in this module's <queries>; Android 11+ hides undeclared packages.
    // Tools that randomise their package name (hidden Magisk, LSPosed parasitic manager) are not visible here.
    static final String[] RISK_PACKAGES={
        "com.topjohnwu.magisk","io.github.vvb2060.magisk","io.github.huskydg.magisk","me.weishu.kernelsu","com.rifsxd.ksunext","me.bmax.apatch",
        "org.lsposed.manager","de.robv.android.xposed.installer","org.meowcat.edxposed.manager","com.solohsu.android.edxp.manager","top.canyie.dreamland.manager","com.tsng.hidemyapplist",
        "moe.shizuku.privileged.api",
        "io.va.exposed","com.lbe.parallel.intl","com.excelliance.dualaid","com.bly.dkplat","com.vmos.pro","com.vphonegaga.titan","com.x8zs.sandbox",
        "org.autojs.autojs","org.autojs.autojspro","org.autojs.autoxjs.v6","com.stardust.scriptdroid","com.cyjh.mobileanjian","com.touchsprite.android","net.dinglisch.android.taskerm","com.llamalab.automate",
        "com.lexa.fakegps","com.incorporateapps.fakegps.fre","com.blogspot.newapphorizons.fakegps","com.theappninjas.fakegpsjoystick",
        "com.guoshi.httpcanary","com.guoshi.httpcanary.premium","app.greyshirts.sslcapture","com.minhui.networkcapture","com.reqable.android","com.emanuelef.remote_capture",
        "catch_.me_.if_.you_.can_",
        "com.teamviewer.quicksupport.market","com.anydesk.anydeskandroid","com.oray.sunlogin"
    };
    private static final String[] HOOK_CLASSES={"de.robv.android.xposed.XposedBridge","de.robv.android.xposed.XposedHelpers","top.canyie.pine.Pine","com.swift.sandhook.SandHook"};
    private static final String[] HOOK_FRAMES={"xposed","lsposed","lsphooker","edxp","sandhook","top.canyie.pine","com.saurik.substrate"};

    static void collect(Context context,JSONObject signals) throws Exception {
        PackageManager pm=context.getPackageManager();
        JSONArray installed=new JSONArray();
        for(String name:RISK_PACKAGES){
            try{pm.getPackageInfo(name,0);installed.put(name);}catch(PackageManager.NameNotFoundException absent){/* absent or hidden */}
        }
        put(signals,"riskApps",new JSONObject().put("installed",installed).put("checked",RISK_PACKAGES.length));

        AccessibilityManager accessibility=(AccessibilityManager)context.getSystemService(Context.ACCESSIBILITY_SERVICE);
        JSONArray services=new JSONArray();
        for(AccessibilityServiceInfo info:accessibility.getEnabledAccessibilityServiceList(AccessibilityServiceInfo.FEEDBACK_ALL_MASK)){
            ServiceInfo service=info.getResolveInfo().serviceInfo;int capabilities=info.getCapabilities();
            services.put(new JSONObject().put("package",service.packageName)
                .put("system",(service.applicationInfo.flags&ApplicationInfo.FLAG_SYSTEM)!=0)
                .put("gestures",(capabilities&AccessibilityServiceInfo.CAPABILITY_CAN_PERFORM_GESTURES)!=0)
                .put("windowContent",(capabilities&AccessibilityServiceInfo.CAPABILITY_CAN_RETRIEVE_WINDOW_CONTENT)!=0));
        }
        // The setting lists user-enabled services even when package visibility filters them from the manager.
        JSONArray configured=new JSONArray();
        String setting=Settings.Secure.getString(context.getContentResolver(),Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES);
        if(setting!=null)for(String component:setting.split(":")){int slash=component.indexOf('/');if(slash>0)configured.put(component.substring(0,slash));}
        put(signals,"accessibility",new JSONObject().put("enabled",accessibility.isEnabled()).put("touchExploration",accessibility.isTouchExplorationEnabled())
            .put("services",services).put("configured",configured));

        put(signals,"automation",new JSONObject().put("monkey",ActivityManager.isUserAMonkey()).put("touches",TouchAudit.snapshot()));

        JSONArray classes=new JSONArray(),frames=new JSONArray();
        for(String name:HOOK_CLASSES){
            for(ClassLoader loader:new ClassLoader[]{ClassLoader.getSystemClassLoader(),context.getClassLoader()}){
                try{Class.forName(name,false,loader);classes.put(name);break;}catch(ClassNotFoundException absent){/* not loaded here */}
            }
        }
        for(StackTraceElement frame:new Throwable().getStackTrace()){
            String text=frame.getClassName().toLowerCase(java.util.Locale.ROOT);
            for(String marker:HOOK_FRAMES)if(text.contains(marker)){frames.put(frame.getClassName());break;}
        }
        put(signals,"javaHooks",new JSONObject().put("classes",classes).put("frames",frames));
    }
    private static void put(JSONObject signals,String name,Object value)throws Exception{signals.put(name,new JSONObject().put("status","available").put("value",value));}
}
