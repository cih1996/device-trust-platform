#if defined(__ANDROID__)
#include <unistd.h>
#include <sys/utsname.h>
#include <sys/system_properties.h>
#include <stdio.h>
#include <errno.h>

static bool jsonText(JNIEnv* env,jobject json,const char* name,const char* value){
    jstring key=env->NewStringUTF(name),text=env->NewStringUTF(value);
    jobject result=call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",key,text);
    if(result)env->DeleteLocalRef(result);if(key)env->DeleteLocalRef(key);if(text)env->DeleteLocalRef(text);return !env->ExceptionCheck();
}
static bool jsonLong(JNIEnv* env,jobject json,const char* name,jlong value){
    jstring key=env->NewStringUTF(name);
    jobject result=call(env,json,"put","(Ljava/lang/String;J)Lorg/json/JSONObject;",key,value);
    if(result)env->DeleteLocalRef(result);if(key)env->DeleteLocalRef(key);return !env->ExceptionCheck();
}
OBF static jstring resources(JNIEnv* env,jclass){
    if(env->EnsureLocalCapacity(256)<0)return nullptr;
    jobject json=construct(env,"org/json/JSONObject","()V");NEED(json);
    jobject errors=construct(env,"org/json/JSONObject","()V");NEED(errors);
    struct utsname kernel;
    if(uname(&kernel)==0){jsonText(env,json,"machine",kernel.machine);jsonText(env,json,"kernelRelease",kernel.release);}
    else jsonText(env,errors,"uname",strerror(errno));
    const char* fields[]={"brand","manufacturer","model","device","hardware","platform"};
    const char* props[]={"ro.product.brand","ro.product.manufacturer","ro.product.model","ro.product.device","ro.hardware","ro.board.platform"};
    for(int i=0;i<6;i++){char value[PROP_VALUE_MAX]={0};if(__system_property_get(props[i],value)>0)jsonText(env,json,fields[i],value);else jsonText(env,errors,fields[i],"property unavailable");}
    long cores=sysconf(_SC_NPROCESSORS_CONF);if(cores>0)jsonLong(env,json,"configuredCores",cores);else jsonText(env,errors,"configuredCores",strerror(errno));
    long page=sysconf(_SC_PAGESIZE);if(page>0)jsonLong(env,json,"pageSize",page);else jsonText(env,errors,"pageSize",strerror(errno));
    FILE* file=fopen("/proc/meminfo","r");
    if(file){char line[512];bool found=false;while(fgets(line,sizeof(line),file)){unsigned long long kb;if(sscanf(line,"MemTotal: %llu kB",&kb)==1){jsonLong(env,json,"memoryBytes",(jlong)(kb*1024));found=true;break;}}fclose(file);if(!found)jsonText(env,errors,"memoryBytes","MemTotal missing");}
    else jsonText(env,errors,"memoryBytes",strerror(errno));
    file=fopen("/proc/cpuinfo","r");
    if(file){char line[512];while(fgets(line,sizeof(line),file)){char* colon=strchr(line,':');if(!colon)continue;*colon=0;char* end=line+strlen(line);while(end>line&&(end[-1]==' '||end[-1]=='\t'))*--end=0;char* value=colon+1;while(*value==' '||*value=='\t')value++;value[strcspn(value,"\r\n")]=0;
        if(strcmp(line,"Hardware")==0)jsonText(env,json,"cpuHardware",value);
        if(strcmp(line,"CPU implementer")==0)jsonText(env,json,"cpuImplementer",value);
        if(strcmp(line,"CPU architecture")==0)jsonText(env,json,"cpuArchitecture",value);
        if(strcmp(line,"model name")==0)jsonText(env,json,"cpuModel",value);
    }fclose(file);}else jsonText(env,errors,"cpuInfo",strerror(errno));
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("errors"),errors);
    return (jstring)call(env,json,"toString","()Ljava/lang/String;");
}
#else
static jstring resources(JNIEnv* env,jclass){error(env,"native resource collection requires Android");return nullptr;}
#endif
