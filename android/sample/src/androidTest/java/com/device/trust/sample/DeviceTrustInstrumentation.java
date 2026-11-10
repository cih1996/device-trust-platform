package com.device.trust.sample;

import android.app.Instrumentation;
import android.os.Bundle;
import java.lang.reflect.InvocationTargetException;

public final class DeviceTrustInstrumentation extends Instrumentation {
 @Override public void onCreate(Bundle args){super.onCreate(args);start();}
 @Override public void onStart(){Bundle result=new Bundle();try{
  Class<?> bridge=getTargetContext().getClassLoader().loadClass("com.device.trust.sample.ProbeBridge");
  String value=(String)bridge.getMethod("run",android.content.Context.class).invoke(null,getTargetContext());
  result.putString("dtp_result",value);finish(-1,result);
 }catch(Throwable error){Throwable cause=error instanceof InvocationTargetException?((InvocationTargetException)error).getCause():error;java.io.StringWriter text=new java.io.StringWriter();cause.printStackTrace(new java.io.PrintWriter(text));result.putString("dtp_error",text.toString());finish(1,result);}}
}
