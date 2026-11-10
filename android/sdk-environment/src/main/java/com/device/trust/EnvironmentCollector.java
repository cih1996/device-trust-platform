package com.device.trust;

import android.Manifest;
import android.content.Context;
import android.content.pm.PackageManager;
import android.location.Location;
import android.location.LocationManager;
import android.net.wifi.WifiInfo;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.os.SystemClock;
import android.telephony.CellInfo;
import android.telephony.CellInfoLte;
import android.telephony.CellInfoGsm;
import android.telephony.CellInfoWcdma;
import android.telephony.CellInfoNr;
import android.telephony.CellIdentityNr;
import android.telephony.TelephonyManager;
import android.util.Log;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.util.List;
import java.util.EnumSet;

/** Optional foreground snapshots. No permission prompts, scans or location subscriptions. */
public final class EnvironmentCollector {
    private EnvironmentCollector() {}
    public enum Collection { WIFI, CELL, LOCATION }

    public static void collect(Context context, JSONObject report, boolean consentGranted, Collection... collections) throws JSONException {
        if (!consentGranted) throw new IllegalStateException("Consent required before collecting environment signals");
        Context app = context.getApplicationContext();
        JSONObject signals = report.getJSONObject("signals");
        EnumSet<Collection> enabled = EnumSet.noneOf(Collection.class);
        for (Collection collection : collections) enabled.add(collection);
        for (Collection collection : enabled) {
            try {
                switch (collection) {
                    case WIFI: wifi(app, signals); break;
                    case CELL: cells(app, signals); break;
                    case LOCATION: location(app, signals); break;
                }
            } catch (SecurityException error) {
                recordError(signals, collection, "permission_denied", error);
            } catch (UnsupportedOperationException error) {
                recordError(signals, collection, "error", error);
            }
        }
    }

    private static void recordError(JSONObject signals, Collection collection, String status, RuntimeException error) throws JSONException {
        Log.e("DeviceTrust",error.toString(),error);
        if (collection == Collection.WIFI) {
            DeviceTrust.status(signals,"bssid",status,error.toString());
            DeviceTrust.status(signals,"wifiRssi",status,error.toString());
        } else DeviceTrust.status(signals,collection == Collection.CELL ? "cell" : "location",status,error.toString());
    }

    @SuppressWarnings("deprecation")
    private static void wifi(Context app, JSONObject signals) throws JSONException {
        if (!app.getPackageManager().hasSystemFeature(PackageManager.FEATURE_WIFI)) {
            DeviceTrust.status(signals,"bssid","unavailable","Wi-Fi hardware not available");
            DeviceTrust.status(signals,"wifiRssi","unavailable","Wi-Fi hardware not available");
            return;
        }
        if (app.checkSelfPermission(Manifest.permission.ACCESS_WIFI_STATE) != PackageManager.PERMISSION_GRANTED) {
            DeviceTrust.status(signals,"bssid","permission_denied",Manifest.permission.ACCESS_WIFI_STATE);
            DeviceTrust.status(signals,"wifiRssi","permission_denied",Manifest.permission.ACCESS_WIFI_STATE);
            return;
        }
        WifiManager manager = (WifiManager) app.getApplicationContext().getSystemService(Context.WIFI_SERVICE);
        WifiInfo info = manager.getConnectionInfo();
        if (info == null) {
            DeviceTrust.status(signals,"bssid","unavailable","no accessible Wi-Fi connection");
            DeviceTrust.status(signals,"wifiRssi","unavailable","no accessible Wi-Fi connection");
            return;
        }
        if (info.getRssi() == -127) DeviceTrust.status(signals,"wifiRssi","unavailable","no RSSI measurement");
        else DeviceTrust.available(signals,"wifiRssi",info.getRssi());
        if (app.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            DeviceTrust.status(signals,"bssid","permission_denied",Manifest.permission.ACCESS_FINE_LOCATION);
            return;
        }
        String bssid = info.getBSSID();
        if (bssid == null || bssid.isEmpty() || "02:00:00:00:00:00".equals(bssid)) {
            DeviceTrust.status(signals,"bssid","unavailable","system returned no accessible BSSID");
        } else DeviceTrust.available(signals,"bssid",bssid);
    }

    private static void cells(Context app, JSONObject signals) throws JSONException {
        if (!app.getPackageManager().hasSystemFeature(PackageManager.FEATURE_TELEPHONY)) {
            DeviceTrust.status(signals,"cell","unavailable","telephony hardware not available");
            return;
        }
        if (app.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            DeviceTrust.status(signals,"cell","permission_denied",Manifest.permission.ACCESS_FINE_LOCATION);
            return;
        }
        TelephonyManager manager = (TelephonyManager) app.getSystemService(Context.TELEPHONY_SERVICE);
        List<CellInfo> cells = manager.getAllCellInfo();
        if (cells == null || cells.isEmpty()) {
            DeviceTrust.status(signals,"cell","unavailable","no cached cell samples");
            return;
        }
        JSONArray samples = new JSONArray();
        for (CellInfo cell : cells) {
            JSONObject sample = new JSONObject().put("registered",cell.isRegistered())
                    .put("ageMillis",sampleAge(cell.getTimeStamp()));
            if (cell instanceof CellInfoLte) {
                CellInfoLte value = (CellInfoLte) cell;
                sample.put("technology","LTE");
                measurement(sample,"cellId",value.getCellIdentity().getCi());
                measurement(sample,"areaCode",value.getCellIdentity().getTac());
                measurement(sample,"dbm",value.getCellSignalStrength().getDbm());
            } else if (cell instanceof CellInfoGsm) {
                CellInfoGsm value = (CellInfoGsm) cell;
                sample.put("technology","GSM");
                measurement(sample,"cellId",value.getCellIdentity().getCid());
                measurement(sample,"areaCode",value.getCellIdentity().getLac());
                measurement(sample,"dbm",value.getCellSignalStrength().getDbm());
            } else if (cell instanceof CellInfoWcdma) {
                CellInfoWcdma value = (CellInfoWcdma) cell;
                sample.put("technology","WCDMA");
                measurement(sample,"cellId",value.getCellIdentity().getCid());
                measurement(sample,"areaCode",value.getCellIdentity().getLac());
                measurement(sample,"dbm",value.getCellSignalStrength().getDbm());
            } else if (Build.VERSION.SDK_INT >= 29 && cell instanceof CellInfoNr) {
                CellInfoNr value = (CellInfoNr) cell;
                CellIdentityNr identity = (CellIdentityNr) value.getCellIdentity();
                sample.put("technology","NR");
                measurement(sample,"cellId",identity.getNci());
                measurement(sample,"areaCode",identity.getTac());
                measurement(sample,"dbm",value.getCellSignalStrength().getDbm());
            } else {
                sample.put("technology",cell.getClass().getSimpleName());
                sample.put("status","unsupported");
            }
            samples.put(sample);
        }
        DeviceTrust.available(signals,"cell",samples);
    }

    private static void measurement(JSONObject sample, String name, long value) throws JSONException {
        sample.put(name,value == Integer.MAX_VALUE || value == Long.MAX_VALUE ? JSONObject.NULL : value);
    }

    private static Object sampleAge(long sampledAtNanos) {
        long now = SystemClock.elapsedRealtimeNanos();
        return sampledAtNanos <= 0 || sampledAtNanos > now ? JSONObject.NULL : (now-sampledAtNanos)/1000000;
    }

    @SuppressWarnings("deprecation")
    private static void location(Context app, JSONObject signals) throws JSONException {
        if (app.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED
                && app.checkSelfPermission(Manifest.permission.ACCESS_COARSE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            DeviceTrust.status(signals,"location","permission_denied","location permission not granted");
            return;
        }
        LocationManager manager = (LocationManager) app.getSystemService(Context.LOCATION_SERVICE);
        Location latest = null;
        for (String provider : manager.getProviders(true)) {
            Location sample = manager.getLastKnownLocation(provider);
            if (sample != null && (latest == null || sample.getElapsedRealtimeNanos() > latest.getElapsedRealtimeNanos())) latest = sample;
        }
        if (latest == null) {
            DeviceTrust.status(signals,"location","unavailable","no cached location");
            return;
        }
        JSONObject value = new JSONObject().put("latitude",latest.getLatitude()).put("longitude",latest.getLongitude())
                .put("timeMillis",latest.getTime()).put("ageMillis",sampleAge(latest.getElapsedRealtimeNanos()))
                .put("accuracyMeters",latest.hasAccuracy()?latest.getAccuracy():JSONObject.NULL)
                .put("mock",Build.VERSION.SDK_INT >= 31 ? latest.isMock() : latest.isFromMockProvider());
        DeviceTrust.available(signals,"location",value);
    }
}
