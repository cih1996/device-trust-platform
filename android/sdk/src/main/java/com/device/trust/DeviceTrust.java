package com.device.trust;

import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.accessibilityservice.AccessibilityServiceInfo;
import android.media.MediaDrm;
import android.media.UnsupportedSchemeException;
import android.net.LinkAddress;
import android.net.LinkProperties;
import android.net.RouteInfo;
import android.net.ProxyInfo;
import android.view.accessibility.AccessibilityManager;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.os.Build;
import android.os.Debug;
import android.os.Looper;
import android.os.Process;
import android.provider.Settings;
import android.telephony.TelephonyManager;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.Inet4Address;
import java.net.InetAddress;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.UUID;

/** Explicit, synchronous API callable from Java and Kotlin. No automatic initialization. */
public final class DeviceTrust {
    private DeviceTrust() {}

    public static final class Options {
        public final String appId;
        /** Device identifiers: ANDROID_ID and a digest of the Widevine device ID. */
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
            drmId(signals);
        } else {
            status(signals,"androidId","disabled","host configuration");
            status(signals,"drmId","disabled","host configuration");
        }
        runtime(app, signals);
        ConnectivityManager cm = (ConnectivityManager) app.getSystemService(Context.CONNECTIVITY_SERVICE);
        Network network = cm.getActiveNetwork();
        NetworkCapabilities caps = network == null ? null : cm.getNetworkCapabilities(network);
        if (caps == null) {
            status(signals,"vpn","unavailable","no active network");
            status(signals,"transport","unavailable","no active network");
            status(signals,"httpProxy","unavailable","no active network");
            status(signals,"activeInterface","unavailable","no active network");
        } else {
            available(signals,"vpn",caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN));
            available(signals,"transport",caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)?"wifi":caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)?"cellular":caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)?"ethernet":"other");
            LinkProperties link = cm.getLinkProperties(network);
            ProxyInfo proxy = link == null ? null : link.getHttpProxy();
            if (proxy == null) proxy = cm.getDefaultProxy();
            if (proxy != null && proxy.getHost() != null && !proxy.getHost().isEmpty()) available(signals,"httpProxy",new JSONObject().put("host",proxy.getHost()).put("port",proxy.getPort()));
            else available(signals,"httpProxy",false);
            // Kernel interface carrying the default network; compared with the claimed transport.
            String iface = link == null ? null : link.getInterfaceName();
            if (iface == null || iface.isEmpty()) status(signals,"activeInterface","unavailable","no interface name");
            else available(signals,"activeInterface",iface);
            available(signals,"networkSnapshot",new JSONObject().put("wifi",caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)).put("cellular",caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)).put("vpn",caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)).put("validated",caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)).put("metered",!caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)));
        }
        lan(cm, signals);
        if (app.getPackageManager().hasSystemFeature(PackageManager.FEATURE_TELEPHONY)) {
            TelephonyManager tm = (TelephonyManager) app.getSystemService(Context.TELEPHONY_SERVICE);
            String operator = tm.getNetworkOperator();
            if (operator == null || operator.isEmpty()) status(signals,"operator","unavailable","no registered network operator");
            else available(signals,"operator",operator);
            // SIM issuer and roaming need no permission; the server compares them with the registered network.
            String sim = tm.getSimOperator();
            if (sim == null || sim.isEmpty()) status(signals,"simOperator","unavailable","no SIM operator");
            else available(signals,"simOperator",sim);
            available(signals,"networkRoaming",tm.isNetworkRoaming());
        } else {
            status(signals,"operator","unavailable","telephony not supported");
            status(signals,"simOperator","unavailable","telephony not supported");
            status(signals,"networkRoaming","unavailable","telephony not supported");
        }
        for (String name : new String[]{"cell", "wifiRssi", "bssid", "location", "installedApps", "bluetoothMac", "routerMac", "attestation"}) {
            status(signals,name,"disabled","not enabled");
        }
        return new JSONObject().put("appId", options.appId).put("installationId",id).put("package",app.getPackageName()).put("signals",signals);
    }

    /**
     * Runtime posture readable without permissions: debuggable build, ADB and developer options,
     * enabled accessibility services (count only), and whether the app runs in a secondary user
     * or a virtual container whose data directory does not belong to this package.
     */
    // Literal /data paths are compared, never opened: they identify a relocated (virtualized) sandbox.
    @android.annotation.SuppressLint("SdCardPath")
    static void runtime(Context app, JSONObject signals) throws JSONException {
        ApplicationInfo info = app.getApplicationInfo();
        available(signals, "appDebuggable", (info.flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
        available(signals, "adbEnabled", Settings.Global.getInt(app.getContentResolver(), Settings.Global.ADB_ENABLED, 0) == 1);
        available(signals, "developerOptions", Settings.Global.getInt(app.getContentResolver(), Settings.Global.DEVELOPMENT_SETTINGS_ENABLED, 0) == 1);
        AccessibilityManager accessibility = (AccessibilityManager) app.getSystemService(Context.ACCESSIBILITY_SERVICE);
        if (accessibility == null) status(signals, "accessibilityServices", "unavailable", "accessibility service missing");
        else available(signals, "accessibilityServices", accessibility.getEnabledAccessibilityServiceList(AccessibilityServiceInfo.FEEDBACK_ALL_MASK).size());
        // Android user ID is uid / 100000 (UserHandle.PER_USER_RANGE). Cloned-app spaces use non-zero users.
        int userId = Process.myUid() / 100000;
        String pkg = app.getPackageName();
        String dataDir = info.dataDir == null ? "" : info.dataDir;
        boolean ownDir = dataDir.equals("/data/user/" + userId + "/" + pkg) || dataDir.equals("/data/data/" + pkg) || dataDir.matches("/mnt/expand/[^/]+/user/" + userId + "/" + java.util.regex.Pattern.quote(pkg));
        available(signals, "appSandbox", new JSONObject().put("userId", userId).put("dataDir", dataDir).put("ownDataDir", ownDir));
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
    /**
     * IPv4 addressing of the Wi-Fi or Ethernet network (not a VPN on top of it): lets the
     * server group handsets behind one router and tell the operator which phone this is.
     */
    @SuppressWarnings("deprecation") // getAllNetworks: the only way to reach the LAN underneath an active VPN.
    private static void lan(ConnectivityManager cm, JSONObject signals) throws JSONException {
        for (Network network : cm.getAllNetworks()) {
            NetworkCapabilities caps = cm.getNetworkCapabilities(network);
            LinkProperties link = cm.getLinkProperties(network);
            if (caps == null || link == null || caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue;
            String transport = caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) ? "wifi" : caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) ? "ethernet" : caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) ? "cellular" : "other";
            String name = link.getInterfaceName();
            // A LAN NIC labelled as cellular (cloud and box phones) still counts; the label is kept.
            boolean lanInterface = name != null && (name.startsWith("eth") || name.startsWith("wlan"));
            if (!transport.equals("wifi") && !transport.equals("ethernet") && !lanInterface) continue;
            JSONArray addresses = new JSONArray(), dns = new JSONArray();
            for (LinkAddress a : link.getLinkAddresses())
                if (a.getAddress() instanceof Inet4Address) addresses.put(new JSONObject().put("address", a.getAddress().getHostAddress()).put("prefix", a.getPrefixLength()));
            String gateway = null;
            for (RouteInfo route : link.getRoutes())
                if (route.isDefaultRoute() && route.getGateway() instanceof Inet4Address) gateway = route.getGateway().getHostAddress();
            for (InetAddress server : link.getDnsServers()) if (server instanceof Inet4Address) dns.put(server.getHostAddress());
            JSONObject value = new JSONObject().put("transport", transport).put("interface", name).put("addresses", addresses).put("gateway", gateway).put("dns", dns);
            if (Build.VERSION.SDK_INT >= 30 && link.getDhcpServerAddress() != null) value.put("dhcpServer", link.getDhcpServerAddress().getHostAddress());
            available(signals, "lan", value);
            return;
        }
        status(signals, "lan", "unavailable", "no Wi-Fi or Ethernet network");
    }
    private static final UUID WIDEVINE = new UUID(0xEDEF8BA979D64ACEL, 0xA3C827DCD51D21EDL);

    /**
     * SHA-256 of the Widevine device unique ID. It survives app reinstall on most devices and
     * lets the server keep one device ID across Keystore keys; the raw value never leaves the phone.
     */
    @SuppressWarnings("deprecation") // MediaDrm.release: close() exists only from API 28.
    private static void drmId(JSONObject signals) throws JSONException {
        MediaDrm drm = null;
        try {
            drm = new MediaDrm(WIDEVINE);
            byte[] id = drm.getPropertyByteArray(MediaDrm.PROPERTY_DEVICE_UNIQUE_ID);
            if (id == null || id.length == 0) status(signals, "drmId", "unavailable", "empty Widevine device ID");
            else available(signals, "drmId", sha256(id));
        } catch (UnsupportedSchemeException e) {
            status(signals, "drmId", "unavailable", "Widevine not supported");
        } catch (RuntimeException e) {
            status(signals, "drmId", "error", e.toString());
        } finally {
            if (drm != null) { if (Build.VERSION.SDK_INT >= 28) drm.close(); else drm.release(); }
        }
    }
    private static String sha256(byte[] data) {
        try {
            byte[] h = MessageDigest.getInstance("SHA-256").digest(data);
            StringBuilder out = new StringBuilder();
            for (byte b : h) out.append(String.format("%02x", b));
            return out.toString();
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException(e);
        }
    }
    static void available(JSONObject signals,String name,Object value) throws JSONException {
        signals.put(name,new JSONObject().put("status","available").put("value",value));
    }
    static void status(JSONObject signals,String name,String state,String reason) throws JSONException {
        signals.put(name,new JSONObject().put("status",state).put("reason",reason));
    }
}
