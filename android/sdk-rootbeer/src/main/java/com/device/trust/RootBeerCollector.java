package com.device.trust;

import android.content.Context;
import android.util.Log;
import com.scottyab.rootbeer.RootBeer;
import com.scottyab.rootbeer.RootBeerNative;
import org.json.JSONException;
import org.json.JSONObject;

/** Optional upstream collector. Results are individual observations, not a trust verdict. */
public final class RootBeerCollector {
    private RootBeerCollector() {}

    public static void collect(Context context, JSONObject report, boolean consentGranted) throws JSONException {
        if (!consentGranted) throw new IllegalStateException("Consent required before collecting root signals");
        JSONObject signals = report.getJSONObject("signals");
        RootBeer rootBeer = new RootBeer(context.getApplicationContext());
        DeviceTrust.available(signals,"rootBeerTestKeys",rootBeer.detectTestKeys());
        DeviceTrust.available(signals,"rootBeerSuBinary",rootBeer.checkForSuBinary());
        DeviceTrust.available(signals,"rootBeerMagiskBinary",rootBeer.checkForMagiskBinary());
        // Load explicitly so upstream's catch-and-false methods cannot hide this failure.
        try {
            System.loadLibrary("toolChecker");
            RootBeerNative nativeCollector = new RootBeerNative();
            String[] paths = {"/system/bin/su","/system/xbin/su","/sbin/su","/vendor/bin/su","/data/local/bin/su","/data/local/xbin/su"};
            DeviceTrust.available(signals,"rootBeerNativeSu",nativeCollector.checkForRoot(paths)>0);
        } catch (UnsatisfiedLinkError error) {
            Log.e("DeviceTrust",error.toString(),error);
            DeviceTrust.status(signals,"rootBeerNativeSu","error",error.toString());
        }
        DeviceTrust.available(signals,"rootBeerVersion","0.1.2");
    }
}
