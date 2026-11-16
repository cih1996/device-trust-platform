// Runtime integrity observations. Every value is a trace that a determined
// attacker can hide; the server treats them as evidence, not as a verdict.
// Paths the app sandbox cannot read are reported as errors, never as clean.
#if defined(__ANDROID__)
#include <dirent.h>
#include <ctype.h>

static bool jsonArrayPut(JNIEnv* env,jobject array,const char* value){
    jobject result=call(env,array,"put","(Ljava/lang/Object;)Lorg/json/JSONArray;",env->NewStringUTF(value));
    if(result)env->DeleteLocalRef(result);return !env->ExceptionCheck();
}
static bool containsFold(const char* text,const char* needle){
    size_t n=strlen(needle);
    for(const char* p=text;*p;p++){size_t i=0;while(i<n&&p[i]&&tolower((unsigned char)p[i])==needle[i])i++;if(i==n)return true;}
    return false;
}

// Library or file-name fragments left in the process address space by hook frameworks.
static const char* const HOOK_MAPS[][2]={
    {"frida","frida"},{"gadget.so","frida"},{"xposed","xposed"},{"edxp","xposed"},{"lspd","lsposed"},{"lsposed","lsposed"},
    {"substrate","substrate"},{"riru","riru"},{"zygisk","zygisk"},{"sandhook","sandhook"}
};
// Thread names created by the Frida agent inside the target process.
static const char* const HOOK_THREADS[]={"gum-js-loop","gmain","gdbus","pool-frida","linjector"};
static const char* const EMULATOR_FILES[]={
    "/dev/qemu_pipe","/dev/goldfish_pipe","/dev/socket/qemud","/sys/qemu_trace","/system/bin/qemu-props",
    "/system/lib/libc_malloc_debug_qemu.so","/dev/vboxguest","/dev/vboxuser","/system/bin/nox-prop",
    "/system/bin/ldinit","/system/lib/libldutils.so","/system/bin/microvirtd","/system/bin/genybaseband"
};
static const char* const EMULATOR_HARDWARE[]={"goldfish","ranchu","vbox86","nox","ttvm","android_x86"};

OBF static jstring integrity(JNIEnv* env,jclass){
    if(env->EnsureLocalCapacity(256)<0)return nullptr;
    jobject json=construct(env,"org/json/JSONObject","()V");NEED(json);
    jobject errors=construct(env,"org/json/JSONObject","()V");NEED(errors);
    jobject hooks=construct(env,"org/json/JSONArray","()V");NEED(hooks);
    jobject threads=construct(env,"org/json/JSONArray","()V");NEED(threads);
    jobject emulator=construct(env,"org/json/JSONArray","()V");NEED(emulator);

    FILE* file=fopen("/proc/self/status","r");
    if(file){char line[256];bool found=false;while(fgets(line,sizeof(line),file)){long pid;if(sscanf(line,"TracerPid: %ld",&pid)==1){jsonLong(env,json,"tracerPid",pid);found=true;break;}}fclose(file);if(!found)jsonText(env,errors,"tracerPid","TracerPid missing");}
    else jsonText(env,errors,"tracerPid",strerror(errno));

    file=fopen("/proc/self/maps","r");
    if(file){
        const size_t kinds=sizeof(HOOK_MAPS)/sizeof(HOOK_MAPS[0]);const char* reported[sizeof(HOOK_MAPS)/sizeof(HOOK_MAPS[0])];size_t count=0;char line[1024];
        while(fgets(line,sizeof(line),file)){const char* path=strchr(line,'/');if(!path)path=strchr(line,'[');if(!path)continue;
            for(size_t i=0;i<kinds;i++){if(!containsFold(path,HOOK_MAPS[i][0]))continue;
                bool duplicate=false;for(size_t j=0;j<count;j++)if(strcmp(reported[j],HOOK_MAPS[i][1])==0)duplicate=true;
                if(!duplicate){reported[count++]=HOOK_MAPS[i][1];jsonArrayPut(env,hooks,HOOK_MAPS[i][1]);}}}
        fclose(file);
    } else jsonText(env,errors,"maps",strerror(errno));

    DIR* tasks=opendir("/proc/self/task");
    if(tasks){struct dirent* entry;char path[64],name[64];
        while((entry=readdir(tasks))!=nullptr){if(entry->d_name[0]=='.')continue;
            snprintf(path,sizeof(path),"/proc/self/task/%s/comm",entry->d_name);FILE* comm=fopen(path,"r");if(!comm)continue;
            if(fgets(name,sizeof(name),comm)){name[strcspn(name,"\r\n")]=0;
                for(const char* marker:HOOK_THREADS)if(strcmp(name,marker)==0){jsonArrayPut(env,threads,marker);break;}}
            fclose(comm);}
        closedir(tasks);
    } else jsonText(env,errors,"threads",strerror(errno));

    char value[PROP_VALUE_MAX];
    if(__system_property_get("ro.kernel.qemu",value)>0&&strcmp(value,"1")==0)jsonArrayPut(env,emulator,"ro.kernel.qemu");
    if(__system_property_get("ro.boot.qemu",value)>0&&strcmp(value,"1")==0)jsonArrayPut(env,emulator,"ro.boot.qemu");
    if(__system_property_get("init.svc.qemud",value)>0)jsonArrayPut(env,emulator,"init.svc.qemud");
    if(__system_property_get("init.svc.qemu-props",value)>0)jsonArrayPut(env,emulator,"init.svc.qemu-props");
    if(__system_property_get("ro.genymotion.version",value)>0)jsonArrayPut(env,emulator,"ro.genymotion.version");
    if(__system_property_get("ro.hardware",value)>0)for(const char* marker:EMULATOR_HARDWARE)if(containsFold(value,marker)){jsonArrayPut(env,emulator,"ro.hardware");break;}
    for(const char* path:EMULATOR_FILES){
        if(access(path,F_OK)==0)jsonArrayPut(env,emulator,path);
        else if(errno!=ENOENT&&errno!=ENOTDIR)jsonText(env,errors,path,strerror(errno));
    }
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("hooks"),hooks);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("hookThreads"),threads);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("emulator"),emulator);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("errors"),errors);
    return (jstring)call(env,json,"toString","()Ljava/lang/String;");
}
#else
static jstring integrity(JNIEnv* env,jclass){error(env,"native integrity collection requires Android");return nullptr;}
#endif
