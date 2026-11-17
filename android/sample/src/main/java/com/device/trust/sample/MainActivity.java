package com.device.trust.sample;

import android.Manifest;
import android.app.Activity;
import android.os.Bundle;
import android.text.InputType;
import android.util.Log;
import android.widget.*;
import android.graphics.Color;
import org.json.JSONObject;
import com.device.trust.DeviceTrust;
import com.device.trust.EnvironmentCollector;
import com.device.trust.RootBeerCollector;
import com.device.trust.secure.PinnedHttp;
import com.device.trust.secure.SecureReporter;
import com.device.trust.secure.ResourceCollector;
import com.device.trust.secure.DeviceTrustClient;
import com.device.trust.secure.TouchAudit;
import android.view.MotionEvent;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;

public class MainActivity extends Activity {
    private TextView status;
    private final ArrayList<Button> controls=new ArrayList<>();
    private String token="",appId="",platform="",business="",eventId="",reportId="",pendingType="";
    private JSONObject challenge,report;
    private CheckBox consent,location;
    private interface Task {void run() throws Exception;}

    @Override public boolean dispatchTouchEvent(MotionEvent event){TouchAudit.record(event);return super.dispatchTouchEvent(event);}
    @Override public void onCreate(Bundle state){
        super.onCreate(state);
        LinearLayout layout=new LinearLayout(this);layout.setOrientation(LinearLayout.VERTICAL);layout.setPadding(32,32,32,32);layout.setFitsSystemWindows(true);
        TextView title=new TextView(this);title.setText("知衡");title.setTextSize(32);title.setTextColor(Color.rgb(20,125,104));layout.addView(title);
        TextView subtitle=new TextView(this);subtitle.setText("业务保护体验");subtitle.setTextSize(16);layout.addView(subtitle);
        EditText service=new EditText(this);service.setHint("服务地址");service.setText(BuildConfig.DEMO_URL);service.setSingleLine(true);layout.addView(service);
        EditText account=new EditText(this);account.setHint("账号");account.setSingleLine(true);layout.addView(account);
        EditText password=new EditText(this);password.setHint("密码");password.setInputType(InputType.TYPE_CLASS_TEXT|InputType.TYPE_TEXT_VARIATION_PASSWORD);layout.addView(password);
        button(layout,"登录",()->{
            String user=account.getText().toString().trim(),pass=password.getText().toString(),url=service.getText().toString().trim();
            task(()->{
                JSONObject result=post(url+"/login",null,new JSONObject().put("accountId",user).put("password",pass));
                token=result.getString("token");appId=result.getString("appId");platform=result.getString("platformUrl");business=url;
                challenge=null;report=null;eventId="";reportId="";
                if(result.has("pending")){eventId=result.getJSONObject("pending").getString("eventId");reportId=result.getJSONObject("pending").getString("reportId");}
                show("已登录 · "+user+(eventId.isEmpty()?"":"\n可查询上次操作"));
            });
        });
        consent=new CheckBox(this);consent.setText("同意本次设备与网络信息采集");layout.addView(consent);
        location=new CheckBox(this);location.setText("包含位置信息");layout.addView(location);
        location.setOnCheckedChangeListener((view,checked)->{if(checked)requestPermissions(new String[]{Manifest.permission.ACCESS_FINE_LOCATION,Manifest.permission.ACCESS_COARSE_LOCATION},1);});
        button(layout,"尝试注册",()->begin("register"));
        button(layout,"领取演示奖励",()->begin("reward"));
        button(layout,"发起演示支付",()->begin("payment"));
        button(layout,"查询并继续",()->task(()->{
            if(eventId.isEmpty())throw new IllegalStateException("暂无待处理请求");
            if(reportId.isEmpty())sendReport();
            finishBusiness();
        }));
        status=new TextView(this);status.setText("准备登记设备");status.setTextSize(15);status.setTextIsSelectable(true);status.setPadding(0,24,0,24);layout.addView(status);
        ScrollView scroll=new ScrollView(this);scroll.addView(layout);setContentView(scroll);
        boolean accepted=getSharedPreferences("dtp_consent",MODE_PRIVATE).getBoolean("accepted",false);
        consent.setChecked(accepted);
        consent.setOnCheckedChangeListener((view,checked)->{
            if(!getSharedPreferences("dtp_consent",MODE_PRIVATE).edit().putBoolean("accepted",checked).commit())throw new IllegalStateException("could not save consent");
            if(checked)initializeDevice();
        });
        button(layout,"更新设备档案",()->{if(!consent.isChecked()){show("请先同意采集");return;}initializeDevice();});
        if(accepted)initializeDevice();else new android.app.AlertDialog.Builder(this).setTitle("设备保护").setMessage("同意采集机型、系统、硬件与网络环境信息，用于设备登记和风险识别。位置信息需另外授权。").setPositiveButton("同意并继续",(dialog,which)->consent.setChecked(true)).setNegativeButton("暂不",(dialog,which)->show("同意采集后可登记设备")).show();
    }
    private void initializeDevice(){boolean includeLocation=location.isChecked();task(()->{
        JSONObject result=DeviceTrustClient.initialize(this,new DeviceTrustClient.Config(BuildConfig.DEMO_APP_ID,BuildConfig.DEMO_URL,BuildConfig.SERVER_PINS),true,(ctx,data)->{
            RootBeerCollector.collect(ctx,data,true);
            EnvironmentCollector.collect(ctx,data,true,EnvironmentCollector.Collection.WIFI,EnvironmentCollector.Collection.CELL);
            if(includeLocation)EnvironmentCollector.collect(ctx,data,true,EnvironmentCollector.Collection.LOCATION);
        });
        show("设备已登记\n设备 ID："+result.getString("deviceId"));
    });}
    private void begin(String kind){
        if(token.isEmpty()){show("请先登录");return;}
        if(!consent.isChecked()){show("请先同意本次采集");return;}
        boolean includeLocation=location.isChecked();
        task(()->{
            String keyId=SecureReporter.keyId(this,appId);
            JSONObject prepared=post(business+"/prepare",token,new JSONObject().put("type",kind).put("keyId",keyId));
            eventId=prepared.getString("eventId");challenge=prepared.getJSONObject("challenge");reportId="";pendingType=kind;
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
        JSONObject result=post(business+"/complete",token,new JSONObject().put("eventId",eventId).put("reportId",reportId));
        String state=result.getString("state");String text;
        if("completed".equals(state))text="操作已完成";
        else if("verify".equals(state))text="需要核验 · 请在工作台处理后继续";
        else if("hold".equals(state))text="等待审核 · 请在工作台处理后继续";
        else throw new IllegalStateException("未知业务状态："+state);
        String deviceId=getSharedPreferences("dtp_secure",MODE_PRIVATE).getString("deviceId:"+appId,"");
        show(text+"\n风险分："+result.getInt("score")+"\n设备 ID："+deviceId+"\n请求号："+eventId.substring(0,12));
    }
    private JSONObject post(String url,String bearer,JSONObject input) throws Exception {
        return PinnedHttp.post(url,BuildConfig.SERVER_PINS,bearer,"application/json",input.toString().getBytes(StandardCharsets.UTF_8));
    }
    private void task(Task task){
        for(Button control:controls)control.setEnabled(false);show("处理中…");
        new Thread(()->{try{task.run();}catch(Exception error){Log.e("DeviceTrust",error.toString(),error);show(error.toString());}catch(LinkageError error){Log.e("DeviceTrust",error.toString(),error);show(error.toString());}finally{runOnUiThread(()->{for(Button control:controls)control.setEnabled(true);});}},"device-trust-request").start();
    }
    private void show(String message){runOnUiThread(()->status.setText(message));}
    private void button(LinearLayout parent,String text,Runnable action){Button button=new Button(this);button.setText(text);parent.addView(button);controls.add(button);button.setOnClickListener(view->action.run());}
}
