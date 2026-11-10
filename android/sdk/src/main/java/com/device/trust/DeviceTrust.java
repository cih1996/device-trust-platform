package com.device.trust;

import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.os.Build;
import android.os.Debug;
import android.os.Looper;
import android.provider.Settings;
import android.telephony.TelephonyManager;
import org.json.JSONException;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.UUID;

/** Explicit, synchronous API callable from Java and Kotlin. No automatic initialization. */
public final class DeviceTrust {
    private DeviceTrust() {}

    public static final class Options {
        public final String appId;
        public final boolean includeAndroidId;
        public Options(String appId, boolean includeAndroidId) {
            if (appId == null || appId.isEmpty()) throw new IllegalArgumentException("appId required");
            this.appId = appId;
            this.includeAndroidId = includeAndroidId;
        }
    }

    /** Call only after the host has obtained consent for this collection. */
    public static JSONObject collect(Context context, Options options, boolean consentGranted) throws JSONException, IOException {
        if (!consentGranted) throw new IllegalStateException("Consent required before collecting device signals");
        Context app = context.getApplicationContext();
        // Host should call collect from a single SDK integration point. ID is installation-scoped.
        SharedPreferences prefs = app.getSharedPreferences("device_trust", Context.MODE_PRIVATE);
        String id = prefs.getString("installation_id", null);
        if (id == null) {
            id = UUID.randomUUID().toString();
            if (!prefs.edit().putString("installation_id", id).commit()) throw new IOException("Could not persist installation ID");
        }
        JSONObject signals = new JSONObject();
        available(signals, "manufacturer", Build.MANUFACTURER);
        available(signals, "model", Build.MODEL);
        available(signals, "sdkInt", Build.VERSION.SDK_INT);
        available(signals, "securityPatch", Build.VERSION.SECURITY_PATCH);
        available(signals, "buildFingerprint", Build.FINGERPRINT);
        available(signals, "debugger", Debug.isDebuggerConnected());
        available(signals, "emulatorHint", Build.FINGERPRINT.startsWith("generic") || Build.MODEL.contains("Emulator") || Build.PRODUCT.contains("sdk_gphone"));
        if (options.includeAndroidId) {
            String value = Settings.Secure.getString(app.getContentResolver(), Settings.Secure.ANDROID_ID);
            if (value == null || value.isEmpty()) status(signals,"androidId","unavailable","system returned no value");
            else available(signals, "androidId", value);
        } else status(signals,"androidId","disabled","host configuration");
        ConnectivityManager cm = (ConnectivityManager) app.getSystemService(Context.CONNECTIVITY_SERVICE);
        Network network = cm.getActiveNetwork();
        NetworkCapabilities caps = network == null ? null : cm.getNetworkCapabilities(network);
        if (caps == null) {
            status(signals,"vpn","unavailable","no active network");
            status(signals,"transport","unavailable","no active network");
        } else {
            available(signals,"vpn",caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN));
            available(signals,"transport",caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)?"wifi":caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)?"cellular":caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)?"ethernet":"other");
            available(signals,"networkSnapshot",new JSONObject().put("wifi",caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)).put("cellular",caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)).put("vpn",caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)).put("validated",caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)).put("metered",!caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)));
        }
        if (app.getPackageManager().hasSystemFeature(PackageManager.FEATURE_TELEPHONY)) {
            TelephonyManager tm = (TelephonyManager) app.getSystemService(Context.TELEPHONY_SERVICE);
            String operator = tm.getNetworkOperator();
            if (operator == null || operator.isEmpty()) status(signals,"operator","unavailable","no registered network operator");
            else available(signals,"operator",operator);
        } else status(signals,"operator","unavailable","telephony not supported");
        for (String name : new String[]{"cell", "wifiRssi", "bssid", "location", "installedApps", "bluetoothMac", "routerMac", "attestation"}) {
            status(signals,name,"disabled","not enabled");
        }
        return new JSONObject().put("appId", options.appId).put("installationId",id).put("package",app.getPackageName()).put("signals",signals);
    }

    /** HTTPS only. Host schedules this call; exceptions retain the server response. */
    public static String upload(String endpoint, JSONObject report) throws IOException, JSONException {
        if (Looper.myLooper() == Looper.getMainLooper()) throw new IllegalStateException("Upload must run off the main thread");
        URL url = new URL(endpoint);
        if (!"https".equals(url.getProtocol())) throw new IllegalArgumentException("HTTPS endpoint required");
        HttpURLConnection connection = (HttpURLConnection) url.openConnection();
        try {
            connection.setConnectTimeout(10000);
            connection.setReadTimeout(15000);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestMethod("POST");
            connection.setRequestProperty("Content-Type","application/json; charset=utf-8");
            connection.setDoOutput(true);
            byte[] bytes = report.toString().getBytes(StandardCharsets.UTF_8);
            connection.setFixedLengthStreamingMode(bytes.length);
            try (java.io.OutputStream stream = connection.getOutputStream()) { stream.write(bytes); }
            int code = connection.getResponseCode();
            InputStream input = code >= 200 && code < 300 ? connection.getInputStream() : connection.getErrorStream();
            String response = "";
            if (input != null) try (InputStream stream = input; ByteArrayOutputStream output = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[4096]; int n;
                while ((n = stream.read(buffer)) != -1) output.write(buffer,0,n);
                response = new String(output.toByteArray(),StandardCharsets.UTF_8);
            }
            if (code != 201) throw new IOException("HTTP " + code + ": " + response);
            return new JSONObject(response).getString("reportId");
        } finally { connection.disconnect(); }
    }
    static void available(JSONObject signals,String name,Object value) throws JSONException {
        signals.put(name,new JSONObject().put("status","available").put("value",value));
    }
    static void status(JSONObject signals,String name,String state,String reason) throws JSONException {
        signals.put(name,new JSONObject().put("status",state).put("reason",reason));
    }
}
