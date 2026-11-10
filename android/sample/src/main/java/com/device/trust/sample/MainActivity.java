package com.device.trust.sample;

import android.Manifest;
import android.app.Activity;
import android.os.Bundle;
import android.util.Log;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import com.device.trust.DeviceTrust;
import com.device.trust.EnvironmentCollector;
import com.device.trust.RootBeerCollector;
import org.json.JSONObject;
import java.util.ArrayList;

public class MainActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        layout.setPadding(32,32,32,32);
        layout.setFitsSystemWindows(true);
        TextView title = new TextView(this); title.setText("采集测试"); title.setTextSize(26); layout.addView(title);
        EditText appId = new EditText(this); appId.setHint("应用 ID"); layout.addView(appId);
        EditText endpoint = new EditText(this); endpoint.setHint("HTTPS 上报地址（可留空）"); layout.addView(endpoint);
        CheckBox wifi = option(layout,"Wi-Fi");
        CheckBox cell = option(layout,"基站与信号");
        CheckBox location = option(layout,"位置缓存");
        CheckBox root = option(layout,"Root 线索");
        CheckBox consent = option(layout,"允许本次采集");
        Button permission = new Button(this); permission.setText("授权位置"); layout.addView(permission);
        permission.setOnClickListener(view -> requestPermissions(new String[]{Manifest.permission.ACCESS_FINE_LOCATION,Manifest.permission.ACCESS_COARSE_LOCATION},1));
        Button collect = new Button(this); collect.setText("开始采集"); layout.addView(collect);
        TextView result = new TextView(this); result.setTextIsSelectable(true); layout.addView(result);
        collect.setOnClickListener(view -> {
            if (!consent.isChecked()) { result.setText("请先允许本次采集"); return; }
            String id = appId.getText().toString().trim();
            String url = endpoint.getText().toString().trim();
            boolean collectRoot = root.isChecked();
            ArrayList<EnvironmentCollector.Collection> groups = new ArrayList<>();
            if (wifi.isChecked()) groups.add(EnvironmentCollector.Collection.WIFI);
            if (cell.isChecked()) groups.add(EnvironmentCollector.Collection.CELL);
            if (location.isChecked()) groups.add(EnvironmentCollector.Collection.LOCATION);
            result.setText("采集中…");
            collect.setEnabled(false);
            new Thread(() -> {
                try {
                    JSONObject report = DeviceTrust.collect(this,new DeviceTrust.Options(id,false),true);
                    EnvironmentCollector.collect(this,report,true,groups.toArray(new EnvironmentCollector.Collection[0]));
                    if (collectRoot) RootBeerCollector.collect(this,report,true);
                    String reportId = url.isEmpty() ? "" : DeviceTrust.upload(url,report);
                    String output = (reportId.isEmpty() ? "" : "报告："+reportId+"\n\n")+report.toString(2);
                    runOnUiThread(() -> result.setText(output));
                } catch (Exception error) {
                    Log.e("DeviceTrustSample",error.toString(),error);
                    runOnUiThread(() -> result.setText(error.toString()));
                } finally {
                    runOnUiThread(() -> collect.setEnabled(true));
                }
            },"device-trust-sample").start();
        });
        ScrollView scroll = new ScrollView(this); scroll.addView(layout); setContentView(scroll);
    }

    private CheckBox option(LinearLayout layout,String label) {
        CheckBox check = new CheckBox(this); check.setText(label); layout.addView(check); return check;
    }
}
