package com.device.trust.secure;

import android.app.Activity;
import android.app.Application;
import android.content.res.Resources;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.DisplayMetrics;
import android.view.MotionEvent;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.util.Calendar;
import java.util.HashMap;

/**
 * Usage statistics for the behaviour profile: sessions, screens, active hours
 * and the "feel" of taps and swipes. Only aggregates since the previous report
 * are kept and uploaded, never coordinates, paths or screen content.
 *
 * Sessions and screens are counted automatically once the SDK is initialized;
 * touches arrive through {@link TouchAudit#record} from dispatchTouchEvent.
 * Single-activity apps call {@link #screen()} when the visible page changes.
 */
public final class BehaviorAudit {
    private BehaviorAudit(){}
    private static final int GRID_X=8,GRID_Y=16;
    private static boolean attached;
    private static long since=System.currentTimeMillis(),foregroundStart;
    private static int started,sessions,screens;
    private static long foregroundMs;
    private static final int[] hours=new int[24];
    // Taps: inter-tap intervals, coarse grid cells, exact repeats, contact size and pressure.
    private static int taps,exactRepeats,intervals;
    private static long lastTapAt;
    private static float lastTapX=-1,lastTapY=-1;
    private static final Stats interval=new Stats(),size=new Stats(),pressure=new Stats();
    private static final HashMap<Integer,Integer> cells=new HashMap<>();
    // Swipes: chord / path length (1 = ruler straight) and speed variation inside each swipe.
    private static int swipes;
    private static double straightness,speedCv;
    private static float downX,downY,prevX,prevY;
    private static long downAt,prevAt;
    private static double path;
    private static final Stats speed=new Stats();

    /** Called by the SDK on initialization. */
    static synchronized void attach(Application app){
        if(attached)return;
        attached=true;
        // Initialization usually runs while the app is in front: that session counts.
        android.app.ActivityManager.RunningAppProcessInfo state=new android.app.ActivityManager.RunningAppProcessInfo();
        android.app.ActivityManager.getMyMemoryState(state);
        if(state.importance==android.app.ActivityManager.RunningAppProcessInfo.IMPORTANCE_FOREGROUND)started(true);
        app.registerActivityLifecycleCallbacks(new Application.ActivityLifecycleCallbacks(){
            @Override public void onActivityStarted(Activity a){started(true);}
            @Override public void onActivityStopped(Activity a){started(false);}
            @Override public void onActivityResumed(Activity a){screen();}
            @Override public void onActivityCreated(Activity a,Bundle b){}
            @Override public void onActivityPaused(Activity a){}
            @Override public void onActivitySaveInstanceState(Activity a,Bundle b){}
            @Override public void onActivityDestroyed(Activity a){}
        });
    }

    /** Counts a page view; Activities are counted automatically. */
    public static synchronized void screen(){screens++;}

    private static synchronized void started(boolean start){
        long now=SystemClock.elapsedRealtime();
        if(start&&started++==0){
            sessions++;foregroundStart=now;
            hours[Calendar.getInstance().get(Calendar.HOUR_OF_DAY)]++;
        } else if(!start&&started>0&&--started==0){
            foregroundMs+=now-foregroundStart;
        }
    }

    static synchronized void record(MotionEvent e){
        long now=e.getEventTime();
        float x=e.getRawX(),y=e.getRawY();
        switch(e.getActionMasked()){
            case MotionEvent.ACTION_DOWN:
                downX=prevX=x;downY=prevY=y;downAt=prevAt=now;path=0;speed.clear();
                size.add(e.getSize());pressure.add(e.getPressure());
                break;
            case MotionEvent.ACTION_MOVE:{
                double step=Math.hypot(x-prevX,y-prevY);long dt=now-prevAt;
                path+=step;
                if(dt>0)speed.add(step/dt);
                prevX=x;prevY=y;prevAt=now;
                break;
            }
            case MotionEvent.ACTION_UP:{
                path+=Math.hypot(x-prevX,y-prevY);
                DisplayMetrics m=Resources.getSystem().getDisplayMetrics();
                if(path<24*m.density&&now-downAt<500)tap(downX,downY,now,m);
                else if(path>0){swipes++;straightness+=Math.hypot(x-downX,y-downY)/path;speedCv+=speed.cv();}
                break;
            }
            default:
        }
    }

    private static void tap(float x,float y,long now,DisplayMetrics m){
        taps++;
        if(lastTapAt>0&&now-lastTapAt<10_000){interval.add(now-lastTapAt);intervals++;}
        if(x==lastTapX&&y==lastTapY)exactRepeats++;
        lastTapAt=now;lastTapX=x;lastTapY=y;
        int cx=Math.min(GRID_X-1,Math.max(0,(int)(x*GRID_X/Math.max(1,m.widthPixels))));
        int cy=Math.min(GRID_Y-1,Math.max(0,(int)(y*GRID_Y/Math.max(1,m.heightPixels))));
        Integer key=cy*GRID_X+cx;Integer n=cells.get(key);cells.put(key,n==null?1:n+1);
    }

    /** Statistics since the previous snapshot; the counters restart. */
    static synchronized JSONObject snapshot() throws JSONException {
        long now=System.currentTimeMillis();
        if(started>0){long t=SystemClock.elapsedRealtime();foregroundMs+=t-foregroundStart;foregroundStart=t;}
        int top=0;for(int n:cells.values())top=Math.max(top,n);
        JSONArray hourly=new JSONArray();for(int h:hours)hourly.put(h);
        JSONObject out=new JSONObject().put("windowMs",now-since).put("sessions",sessions).put("foregroundMs",foregroundMs).put("screens",screens).put("hours",hourly)
            .put("taps",new JSONObject().put("count",taps).put("intervalMeanMs",round(interval.mean())).put("intervalCv",round(interval.cv())).put("intervals",intervals)
                .put("cells",cells.size()).put("topCellShare",taps==0?0:round((double)top/taps)).put("exactRepeat",taps==0?0:round((double)exactRepeats/taps))
                .put("sizeCv",round(size.cv())).put("pressureCv",round(pressure.cv())))
            .put("swipes",new JSONObject().put("count",swipes).put("straightness",swipes==0?0:round(straightness/swipes)).put("speedCv",swipes==0?0:round(speedCv/swipes)));
        since=now;sessions=screens=taps=exactRepeats=intervals=swipes=0;foregroundMs=0;straightness=speedCv=0;
        java.util.Arrays.fill(hours,0);cells.clear();interval.clear();size.clear();pressure.clear();
        return out;
    }

    private static double round(double v){return Math.round(v*1000)/1000.0;}

    private static final class Stats {
        private int n;private double sum,sq;
        void add(double v){n++;sum+=v;sq+=v*v;}
        void clear(){n=0;sum=sq=0;}
        double mean(){return n==0?0:sum/n;}
        double cv(){if(n<2)return 0;double m=sum/n,var=Math.max(0,sq/n-m*m);return m==0?0:Math.sqrt(var)/m;}
    }
}
