package com.device.trust.sample;

import android.Manifest;
import android.app.Activity;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Bundle;
import android.text.InputType;
import android.util.Log;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.View;
import android.widget.*;
import com.device.trust.DeviceTrust;
import com.device.trust.EnvironmentCollector;
import com.device.trust.RootBeerCollector;
import com.device.trust.secure.DeviceTrustClient;
import com.device.trust.secure.PinnedHttp;
import com.device.trust.secure.ResourceCollector;
import com.device.trust.secure.SecureReporter;
import com.device.trust.secure.TouchAudit;
import org.json.JSONArray;
import org.json.JSONObject;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;

/**
 * The online experience app. Three tabs: the device's risk as the platform
 * sees it, protected demo business operations with their scoring, and how to
 * view the results in the console.
 */
public class MainActivity extends Activity {
    // Public experience console account; it can view and resolve events but not add apps.
    static final String CONSOLE_URL="https://devicetrust-console.lingzidev.com/",CONSOLE_ACCOUNT="demo-android",CONSOLE_PASSWORD="zhiheng2026";
    private static final int ACCENT=Color.rgb(20,125,104),MUTED=Color.rgb(110,116,122),LINE=Color.rgb(228,231,234);

    private final ArrayList<Button> controls=new ArrayList<>();
    private final ArrayList<View> pages=new ArrayList<>();
    private final ArrayList<TextView> tabs=new ArrayList<>();
    private String token="",appId=BuildConfig.DEMO_APP_ID,platform="",eventId="",reportId="";
    private JSONObject challenge,report;
    private CheckBox consent,location;
    private EditText service;
    private TextView status,score,level,summary,businessStatus,businessScore;
    private LinearLayout factors,businessHits;
    private interface Task {void run() throws Exception;}

    @Override public boolean dispatchTouchEvent(MotionEvent event){TouchAudit.record(event);return super.dispatchTouchEvent(event);}

    @Override public void onCreate(Bundle state){
        super.onCreate(state);
        LinearLayout root=new LinearLayout(this);root.setOrientation(LinearLayout.VERTICAL);root.setFitsSystemWindows(true);root.setBackgroundColor(Color.rgb(246,247,248));
        TextView title=text("知衡 · 风控体验",22,Color.BLACK);title.setTypeface(Typeface.DEFAULT_BOLD);title.setPadding(40,32,40,16);root.addView(title);
        FrameLayout content=new FrameLayout(this);root.addView(content,new LinearLayout.LayoutParams(-1,0,1));
        content.addView(scroll(devicePage()));content.addView(scroll(businessPage()));content.addView(scroll(aboutPage()));
        LinearLayout bar=new LinearLayout(this);bar.setBackgroundColor(Color.WHITE);root.addView(bar,new LinearLayout.LayoutParams(-1,-2));
        String[] names={"设备","业务","说明"};
        for(int i=0;i<names.length;i++){final int index=i;TextView tab=text(names[i],15,MUTED);tab.setGravity(Gravity.CENTER);tab.setPadding(0,28,0,28);tab.setOnClickListener(v->select(index));bar.addView(tab,new LinearLayout.LayoutParams(0,-2,1));tabs.add(tab);}
        setContentView(root);select(0);
        boolean accepted=getSharedPreferences("dtp_consent",MODE_PRIVATE).getBoolean("accepted",false);
        consent.setChecked(accepted);
        consent.setOnCheckedChangeListener((view,checked)->{
            if(!getSharedPreferences("dtp_consent",MODE_PRIVATE).edit().putBoolean("accepted",checked).commit())throw new IllegalStateException("could not save consent");
            if(checked)initializeDevice();
        });
        if(accepted)initializeDevice();else new android.app.AlertDialog.Builder(this).setTitle("设备保护").setMessage("同意采集机型、系统、硬件与网络环境信息，用于设备登记和风险识别。位置信息需另外授权。").setPositiveButton("同意并继续",(dialog,which)->consent.setChecked(true)).setNegativeButton("暂不",(dialog,which)->show("同意采集后可登记设备")).show();
    }

    private View devicePage(){
        LinearLayout page=column();
        LinearLayout card=card(page);
        card.addView(text("设备风险分",14,MUTED));
        score=text("--",56,Color.BLACK);score.setTypeface(Typeface.DEFAULT_BOLD);card.addView(score);
        level=text("尚未登记",15,MUTED);card.addView(level);
        summary=text("",13,MUTED);summary.setPadding(0,12,0,0);summary.setTextIsSelectable(true);card.addView(summary);
        LinearLayout list=card(page);
        list.addView(heading("评分条目"));
        factors=column();list.addView(factors);
        LinearLayout actions=card(page);
        consent=new CheckBox(this);consent.setText("同意设备与网络信息采集");actions.addView(consent);
        location=new CheckBox(this);location.setText("包含位置信息");actions.addView(location);
        location.setOnCheckedChangeListener((view,checked)->{if(checked)requestPermissions(new String[]{Manifest.permission.ACCESS_FINE_LOCATION,Manifest.permission.ACCESS_COARSE_LOCATION},1);});
        button(actions,"更新设备档案",()->{if(!consent.isChecked()){show("请先同意采集");return;}initializeDevice();});
        button(actions,"刷新风险分",()->task(this::loadRisk));
        status=text("准备登记设备",13,MUTED);status.setPadding(0,16,0,0);status.setTextIsSelectable(true);actions.addView(status);
        return page;
    }

    private View businessPage(){
        LinearLayout page=column();
        LinearLayout login=card(page);
        login.addView(heading("演示业务账号"));
        service=new EditText(this);service.setHint("服务地址");service.setText(BuildConfig.DEMO_URL);service.setSingleLine(true);login.addView(service);
        EditText account=new EditText(this);account.setHint("账号");account.setText("体验用户");account.setSingleLine(true);login.addView(account);
        EditText password=new EditText(this);password.setHint("密码");password.setText(BuildConfig.DEMO_PASSWORD);password.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);login.addView(password);
        button(login,"登录",()->{
            String user=account.getText().toString().trim(),pass=password.getText().toString();
            task(()->{
                JSONObject result=post("/login",null,new JSONObject().put("accountId",user).put("password",pass));
                token=result.getString("token");appId=result.getString("appId");platform=result.getString("platformUrl");
                challenge=null;report=null;eventId="";reportId="";
                if(result.has("pending")){eventId=result.getJSONObject("pending").getString("eventId");reportId=result.getJSONObject("pending").getString("reportId");}
                businessShow("已登录 · "+user+(eventId.isEmpty()?"":"\n有一笔待处理的操作，可点“查询并继续”"));
            });
        });
        LinearLayout ops=card(page);
        ops.addView(heading("受保护的操作"));
        button(ops,"尝试注册",()->begin("register"));
        button(ops,"领取演示奖励",()->begin("reward"));
        button(ops,"发起演示支付",()->begin("payment"));
        button(ops,"查询并继续",()->task(()->{
            if(eventId.isEmpty())throw new IllegalStateException("暂无待处理请求");
            if(reportId.isEmpty())sendReport();
            finishBusiness();
        }));
        LinearLayout result=card(page);
        result.addView(heading("本次评估"));
        businessScore=text("--",40,Color.BLACK);businessScore.setTypeface(Typeface.DEFAULT_BOLD);result.addView(businessScore);
        businessStatus=text("登录后选择一项操作",14,MUTED);businessStatus.setTextIsSelectable(true);result.addView(businessStatus);
        businessHits=column();businessHits.setPadding(0,16,0,0);result.addView(businessHits);
        return page;
    }

    private View aboutPage(){
        LinearLayout page=column();
        LinearLayout console=card(page);
        console.addView(heading("在工作台查看"));
        TextView info=text("网址 "+CONSOLE_URL+"\n公开体验账号 "+CONSOLE_ACCOUNT+"\n密码 "+CONSOLE_PASSWORD+"\n\n可查看设备档案、事件和评分明细，并处理“需要核验 / 等待审核”的操作。所有体验者共用此账号，数据互相可见。",14,Color.DKGRAY);
        info.setTextIsSelectable(true);console.addView(info);
        LinearLayout how=card(page);
        how.addView(heading("怎么体验"));
        how.addView(text("1. 设备页：同意采集后自动登记，显示平台对这台手机的风险分和每一条评分依据。\n2. 打开 USB 调试、开代理或 VPN、装 Root / Hook 工具后点“更新设备档案”，看分数变化。\n3. 业务页：登录后尝试注册、领奖、支付，查看每次操作的评估结果；高风险操作会要求在工作台核验。",14,Color.DKGRAY));
        how.addView(text("\n应用版本 "+BuildConfig.VERSION_NAME,12,MUTED));
        return page;
    }

    private void select(int index){
        for(int i=0;i<pages.size();i++){pages.get(i).setVisibility(i==index?View.VISIBLE:View.GONE);tabs.get(i).setTextColor(i==index?ACCENT:MUTED);tabs.get(i).setTypeface(i==index?Typeface.DEFAULT_BOLD:Typeface.DEFAULT);}
    }

    private void initializeDevice(){boolean includeLocation=location.isChecked();task(()->{
        DeviceTrustClient.initialize(this,new DeviceTrustClient.Config(BuildConfig.DEMO_APP_ID,BuildConfig.DEMO_URL,BuildConfig.SERVER_PINS),true,(ctx,data)->{
            RootBeerCollector.collect(ctx,data,true);
            EnvironmentCollector.collect(ctx,data,true,EnvironmentCollector.Collection.WIFI,EnvironmentCollector.Collection.CELL);
            if(includeLocation)EnvironmentCollector.collect(ctx,data,true,EnvironmentCollector.Collection.LOCATION);
        });
        show("设备已登记");
        loadRisk();
    });}

    /** The platform's current view of this device: score, level and factors. */
    private void loadRisk() throws Exception {
        String deviceId=SecureReporter.deviceId(this,BuildConfig.DEMO_APP_ID);
        if(deviceId.isEmpty())throw new IllegalStateException("设备尚未登记");
        JSONObject v=post("/device/risk",null,new JSONObject().put("deviceId",deviceId));
        runOnUiThread(()->{
            try {
                int risk=v.getInt("riskScore");
                score.setText(String.valueOf(risk));score.setTextColor(levelColor(v.getString("riskLevel")));
                level.setText(levelName(v.getString("riskLevel"))+" · 环境 "+v.getInt("envScore")+" + 业务 "+v.getInt("businessScore"));
                JSONObject app=v.getJSONObject("app");
                String proof=app.optString("attestation","").isEmpty()?"仅签名（无硬件证明）":"硬件证明通过（"+app.getString("attestation")+"）";
                summary.setText("设备 ID "+deviceId.substring(0,16)+"…\n"+proof+"\n出口 IP "+v.optString("observedIp"));
                fillHits(factors,v.optJSONArray("factors"),"环境干净，没有命中评分项");
            } catch(Exception e){show(e.toString());}
        });
        show("已更新");
    }

    private void begin(String kind){
        if(token.isEmpty()){businessShow("请先登录");return;}
        if(!consent.isChecked()){businessShow("请先在设备页同意采集");return;}
        boolean includeLocation=location.isChecked();
        task(()->{
            String keyId=SecureReporter.keyId(this,appId);
            JSONObject prepared=post("/prepare",token,new JSONObject().put("type",kind).put("keyId",keyId));
            eventId=prepared.getString("eventId");challenge=prepared.getJSONObject("challenge");reportId="";
            report=DeviceTrust.collect(this,new DeviceTrust.Options(appId,true),true);
            ResourceCollector.collect(this,report,true);
            if(includeLocation)EnvironmentCollector.collect(this,report,true,EnvironmentCollector.Collection.WIFI,EnvironmentCollector.Collection.CELL,EnvironmentCollector.Collection.LOCATION);
            RootBeerCollector.collect(this,report,true);
            sendReport();finishBusiness();
        });
    }

    private void sendReport() throws Exception {
        if(challenge==null||report==null)throw new IllegalStateException("本次请求需要重新发起");
        JSONObject result=SecureReporter.upload(this,platform,BuildConfig.SERVER_PINS,challenge,report);
        reportId=result.getString("reportId");
    }

    private void finishBusiness() throws Exception {
        JSONObject result=post("/complete",token,new JSONObject().put("eventId",eventId).put("reportId",reportId));
        String state=result.getString("state");String text;
        if("completed".equals(state))text="操作已完成";
        else if("verify".equals(state))text="需要核验 · 请在工作台处理后点“查询并继续”";
        else if("hold".equals(state))text="等待审核 · 请在工作台处理后点“查询并继续”";
        else throw new IllegalStateException("未知业务状态："+state);
        int points=result.getInt("score");
        runOnUiThread(()->{
            businessScore.setText(String.valueOf(points));businessScore.setTextColor(points>=70?Color.rgb(196,50,50):points>=40?Color.rgb(205,120,20):ACCENT);
            businessStatus.setText(text+"\n请求号 "+eventId.substring(0,12));
            fillHits(businessHits,result.optJSONArray("hits"),"没有命中评分项");
        });
        loadRisk();
    }

    private void fillHits(LinearLayout list,JSONArray hits,String empty){
        list.removeAllViews();
        int shown=0;
        for(int i=0;hits!=null&&i<hits.length();i++){
            JSONObject hit=hits.optJSONObject(i);if(hit==null||hit.optInt("points")==0)continue;
            int points=hit.optInt("points");
            LinearLayout row=new LinearLayout(this);row.setPadding(0,14,0,14);
            String name=hit.optString("label","");if(name.isEmpty())name=hit.optString("code");
            TextView label=text(name,15,Color.BLACK);row.addView(label,new LinearLayout.LayoutParams(0,-2,1));
            TextView value=text(points>0?"+"+points:String.valueOf(points),15,points>0?Color.rgb(196,50,50):ACCENT);value.setTypeface(Typeface.DEFAULT_BOLD);row.addView(value);
            list.addView(row);
            View rule=new View(this);rule.setBackgroundColor(LINE);list.addView(rule,new LinearLayout.LayoutParams(-1,1));
            shown++;
        }
        if(shown==0)list.addView(text(empty,14,MUTED));
    }

    private static String levelName(String level){return "high".equals(level)?"高风险":"medium".equals(level)?"中风险":"低风险";}
    private static int levelColor(String level){return "high".equals(level)?Color.rgb(196,50,50):"medium".equals(level)?Color.rgb(205,120,20):ACCENT;}

    private JSONObject post(String path,String bearer,JSONObject input) throws Exception {
        String base=service==null?BuildConfig.DEMO_URL:service.getText().toString().trim();
        return PinnedHttp.post(base+path,BuildConfig.SERVER_PINS,bearer,"application/json",input.toString().getBytes(StandardCharsets.UTF_8));
    }
    private void task(Task task){
        for(Button control:controls)control.setEnabled(false);show("处理中…");
        new Thread(()->{try{task.run();}catch(Exception error){Log.e("DeviceTrust",error.toString(),error);show(error.toString());businessShow(error.toString());}catch(LinkageError error){Log.e("DeviceTrust",error.toString(),error);show(error.toString());}finally{runOnUiThread(()->{for(Button control:controls)control.setEnabled(true);});}},"device-trust-request").start();
    }
    private void show(String message){runOnUiThread(()->status.setText(message));}
    private void businessShow(String message){runOnUiThread(()->businessStatus.setText(message));}

    private ScrollView scroll(View page){ScrollView scroll=new ScrollView(this);scroll.addView(page);pages.add(scroll);return scroll;}
    private LinearLayout column(){LinearLayout l=new LinearLayout(this);l.setOrientation(LinearLayout.VERTICAL);return l;}
    private LinearLayout card(LinearLayout parent){
        LinearLayout c=column();c.setPadding(40,32,40,32);
        GradientDrawable bg=new GradientDrawable();bg.setColor(Color.WHITE);bg.setCornerRadius(24);c.setBackground(bg);
        LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.setMargins(32,0,32,24);parent.addView(c,p);return c;
    }
    private TextView heading(String s){TextView t=text(s,16,Color.BLACK);t.setTypeface(Typeface.DEFAULT_BOLD);t.setPadding(0,0,0,8);return t;}
    private TextView text(String s,int size,int color){TextView t=new TextView(this);t.setText(s);t.setTextSize(size);t.setTextColor(color);return t;}
    private void button(LinearLayout parent,String text,Runnable action){Button button=new Button(this);button.setText(text);parent.addView(button);controls.add(button);button.setOnClickListener(view->action.run());}
}
