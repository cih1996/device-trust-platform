package com.device.trust.secure;

import android.view.InputDevice;
import android.view.MotionEvent;
import org.json.JSONObject;

/**
 * Counts touch-down events that did not come from a touchscreen finger. Call {@link #record}
 * from Activity.dispatchTouchEvent. Events injected by the input shell command, instrumentation
 * or accessibility gestures carry pressure 1, size 1 and no contact ellipse; a real panel
 * reports a measured contact area.
 */
public final class TouchAudit {
    private TouchAudit(){}
    private static int downs,synthetic,virtualDevice;
    public static synchronized void record(MotionEvent event){
        if(event.getActionMasked()!=MotionEvent.ACTION_DOWN)return;
        downs++;
        if(event.getPressure()==1f&&event.getSize()==1f&&event.getTouchMajor()==0f&&event.getTouchMinor()==0f)synthetic++;
        InputDevice device=event.getDevice();
        if(device==null||device.isVirtual())virtualDevice++;
    }
    static synchronized JSONObject snapshot() throws org.json.JSONException {
        return new JSONObject().put("downs",downs).put("synthetic",synthetic).put("virtualDevice",virtualDevice);
    }
}
