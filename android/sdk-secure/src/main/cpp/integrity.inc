// Runtime integrity observations. Every value is a trace that a determined
// attacker can hide; the server treats them as evidence, not as a verdict.
// Paths the app sandbox cannot read are reported as errors, never as clean.
#if defined(__ANDROID__)
#include <dirent.h>
#include <ctype.h>
#include <ifaddrs.h>
#include <dlfcn.h>
#include <net/if.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <fcntl.h>
#include <poll.h>
#include <sys/socket.h>
#include <time.h>
#include <sys/syscall.h>
#include <pthread.h>

static bool jsonArrayPut(JNIEnv* env,jobject array,const char* value){
    jstring text=env->NewStringUTF(value);
    jobject result=call(env,array,"put","(Ljava/lang/Object;)Lorg/json/JSONArray;",text);
    if(result)env->DeleteLocalRef(result);if(text)env->DeleteLocalRef(text);return !env->ExceptionCheck();
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


// Property names left by root and hook frameworks. Names only; values are not reported.
static const char* const ROOT_PROPS[]={"magisk","kernelsu","ksu.","apatch","zygisk","lsposed","xposed","riru","shamiko","playintegrity","trickystore","tricky_store"};
// Boot and bootloader state. Hiding tools rewrite ro.boot.verifiedbootstate but often miss the copies.
static const char* const BOOT_PROPS[]={"verifiedbootstate","vbmeta_state","vbmeta.device_state","flash.locked","veritymode"};
// Mount sources or targets that only appear when root overlays are visible to this process.
static const char* const ROOT_MOUNTS[]={"magisk","/data/adb","kernelsu","ksu","apatch","zygisk","/debug_ramdisk"};

struct PropScan{JNIEnv* env;jobject boot,root;size_t bootCount,rootCount;};
static void propMatch(PropScan* scan,const char* name,const char* value){
    if(scan->env->ExceptionCheck())return;
    for(const char* marker:BOOT_PROPS)if(containsFold(name,marker)&&scan->bootCount<32){jsonText(scan->env,scan->boot,name,value);scan->bootCount++;break;}
    for(const char* marker:ROOT_PROPS)if(containsFold(name,marker)&&scan->rootCount<32){jsonArrayPut(scan->env,scan->root,name);scan->rootCount++;break;}
}
// API 26+ exposes full-length names through the callback reader; the legacy reader truncates at 32 bytes.
typedef void (*PropReadCallback)(const prop_info*,void (*)(void*,const char*,const char*,uint32_t),void*);
static PropReadCallback propReader;
static void propReceived(void* cookie,const char* name,const char* value,uint32_t){propMatch((PropScan*)cookie,name,value);}
static void propVisit(const prop_info* info,void* cookie){
    if(propReader){propReader(info,propReceived,cookie);return;}
    char name[PROP_NAME_MAX],value[PROP_VALUE_MAX];
    if(__system_property_read(info,name,value)>=0)propMatch((PropScan*)cookie,name,value);
}

// Executable code that neither an installed package nor the system image provides:
// injected agents load from /data/local/tmp or from memfd and delete the backing file.
// ART's JIT caches (memfd:jit-cache, ashmem jit-zygote-cache) are excluded.
enum {INJECT_NONE,INJECT_TMP,INJECT_MEMFD,INJECT_DELETED};
static int injectedKind(const char* line,const char* path){
    const char* perms=strchr(line,' ');if(!perms||strlen(perms)<4||perms[3]!='x')return INJECT_NONE;
    if(containsFold(path,"jit-"))return INJECT_NONE;
    if(strstr(path,"/data/local/tmp/"))return INJECT_TMP;
    if(strncmp(path,"/memfd:",7)==0)return INJECT_MEMFD;
    if(strncmp(path,"/data/",6)==0&&strstr(path,"(deleted)"))return INJECT_DELETED;
    return INJECT_NONE;
}

static bool jsonArrayLong(JNIEnv* env,jobject array,jlong value){
    jobject result=call(env,array,"put","(J)Lorg/json/JSONArray;",value);
    if(result)env->DeleteLocalRef(result);return !env->ExceptionCheck();
}
static long elapsedMillis(const struct timespec* start){
    struct timespec now;clock_gettime(CLOCK_MONOTONIC,&now);
    return (now.tv_sec-start->tv_sec)*1000+(now.tv_nsec-start->tv_nsec)/1000000;
}
// Apps cannot read /proc/net on Android 10+, so listeners are found by connecting.
// Loopback connects resolve inside the kernel; refused ports fail immediately.
// Bionic routes socket() and connect() through netd for network marking, one IPC
// per call; loopback traffic needs no mark, so the scan issues the system calls directly.
static int rawSocket(){return (int)syscall(__NR_socket,AF_INET,SOCK_STREAM|SOCK_NONBLOCK|SOCK_CLOEXEC,0);}
static int rawConnect(int fd,const struct sockaddr_in* addr){return (int)syscall(__NR_connect,fd,addr,sizeof(*addr));}
struct ScanRange{int first,last,found,failure;uint16_t ports[64];};
static void* scanRange(void* argument){
    ScanRange* range=(ScanRange*)argument;
    enum{BATCH=256};
    for(int start=range->first;start<=range->last;start+=BATCH){
        struct pollfd pending[BATCH];uint16_t numbers[BATCH];int n=0;
        for(int port=start;port<start+BATCH&&port<=range->last;port++){
            int fd=rawSocket();
            if(fd<0){range->failure=errno;for(int i=0;i<n;i++)close(pending[i].fd);return nullptr;}
            struct sockaddr_in addr;memset(&addr,0,sizeof(addr));addr.sin_family=AF_INET;addr.sin_port=htons((uint16_t)port);addr.sin_addr.s_addr=htonl(INADDR_LOOPBACK);
            if(rawConnect(fd,&addr)==0){if(range->found<64)range->ports[range->found]=(uint16_t)port;range->found++;close(fd);continue;}
            if(errno!=EINPROGRESS){close(fd);continue;}
            pending[n].fd=fd;pending[n].events=POLLOUT;pending[n].revents=0;numbers[n]=(uint16_t)port;n++;
        }
        int left=n;
        while(left>0&&poll(pending,n,50)>0){
            for(int i=0;i<n;i++){if(pending[i].fd<0||!pending[i].revents)continue;
                int error=0;socklen_t size=sizeof(error);
                if(getsockopt(pending[i].fd,SOL_SOCKET,SO_ERROR,&error,&size)==0&&error==0){if(range->found<64)range->ports[range->found]=numbers[i];range->found++;}
                close(pending[i].fd);pending[i].fd=-1;left--;}
        }
        for(int i=0;i<n;i++)if(pending[i].fd>=0)close(pending[i].fd);
    }
    return nullptr;
}
// Socket setup and teardown dominate, so the port range is split across threads.
static int scanLoopback(uint16_t* ports,int cap,int* failure){
    enum{THREADS=8};ScanRange ranges[THREADS];pthread_t threads[THREADS];bool started[THREADS];
    int span=(65535+THREADS-1)/THREADS;
    for(int t=0;t<THREADS;t++){
        ranges[t].first=1+t*span;ranges[t].last=t==THREADS-1?65535:(t+1)*span;ranges[t].found=0;ranges[t].failure=0;
        int error=pthread_create(&threads[t],nullptr,scanRange,&ranges[t]);
        started[t]=error==0;if(error)ranges[t].failure=error;
    }
    int found=0;
    for(int t=0;t<THREADS;t++){
        if(started[t])pthread_join(threads[t],nullptr);
        if(ranges[t].failure)*failure=ranges[t].failure;
        for(int i=0;i<ranges[t].found&&i<64;i++)if(found<cap)ports[found++]=ranges[t].ports[i];else found++;
        if(ranges[t].found>64)found+=ranges[t].found-64;
    }
    return found;
}
// address is in network byte order. Returns a connected non-blocking socket or -1.
static int connectTimed(uint32_t address,uint16_t port,int timeoutMs){
    int fd=socket(AF_INET,SOCK_STREAM|SOCK_NONBLOCK|SOCK_CLOEXEC,0);if(fd<0)return -1;
    struct sockaddr_in addr;memset(&addr,0,sizeof(addr));addr.sin_family=AF_INET;addr.sin_port=htons(port);addr.sin_addr.s_addr=address;
    if(connect(fd,(struct sockaddr*)&addr,sizeof(addr))==0)return fd;
    if(errno!=EINPROGRESS){close(fd);return -1;}
    struct pollfd p={fd,POLLOUT,0};int error=0;socklen_t size=sizeof(error);
    if(poll(&p,1,timeoutMs)!=1||getsockopt(fd,SOL_SOCKET,SO_ERROR,&error,&size)!=0||error!=0){close(fd);return -1;}
    return fd;
}
// Sends one request to every port at once and collects replies until a shared deadline.
enum{PROBE_MAX=32,PROBE_REPLY=512};
static void probeAll(const uint16_t* ports,int n,const char* request,size_t length,char replies[][PROBE_REPLY],int* sizes){
    struct pollfd fds[PROBE_MAX];
    for(int i=0;i<n;i++){sizes[i]=0;replies[i][0]=0;
        int fd=connectTimed(htonl(INADDR_LOOPBACK),ports[i],100);
        if(fd>=0&&send(fd,request,length,MSG_NOSIGNAL)!=(ssize_t)length){close(fd);fd=-1;}
        fds[i].fd=fd;fds[i].events=POLLIN;fds[i].revents=0;}
    struct timespec start;clock_gettime(CLOCK_MONOTONIC,&start);
    for(long left=200;left>0;left=200-elapsedMillis(&start)){
        if(poll(fds,n,(int)left)<=0)break;
        for(int i=0;i<n;i++){if(fds[i].fd<0||!fds[i].revents)continue;
            ssize_t got=recv(fds[i].fd,replies[i]+sizes[i],PROBE_REPLY-1-sizes[i],0);
            if(got<=0||sizes[i]+got>=PROBE_REPLY-1){if(got>0)sizes[i]+=(int)got;close(fds[i].fd);fds[i].fd=-1;}
            else sizes[i]+=(int)got;
            replies[i][sizes[i]]=0;}
    }
    for(int i=0;i<n;i++)if(fds[i].fd>=0)close(fds[i].fd);
}
// Frida's server answers a D-Bus AUTH line with REJECTED (classic protocol) and names
// itself in HTTP responses (WebSocket protocol). Either marks the listener as Frida.
static void fridaListeners(const uint16_t* ports,int n,bool* frida){
    char replies[PROBE_MAX][PROBE_REPLY];int sizes[PROBE_MAX];
    const char auth[7]={0,'A','U','T','H','\r','\n'};
    probeAll(ports,n,auth,sizeof(auth),replies,sizes);
    for(int i=0;i<n;i++)frida[i]=sizes[i]>=8&&memcmp(replies[i],"REJECTED",8)==0;
    const char http[]="GET / HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n";
    probeAll(ports,n,http,sizeof(http)-1,replies,sizes);
    for(int i=0;i<n;i++)frida[i]=frida[i]||(sizes[i]>0&&containsFold(replies[i],"frida"));
}
// Address class only: the server needs to know which interfaces carry traffic, not the addresses.
// Returns a code; string literals are produced at the use site because string encryption
// decrypts them into storage that does not outlive the function that names them.
enum {SCOPE_LOOPBACK,SCOPE_LINKLOCAL,SCOPE_PRIVATE,SCOPE_CGNAT,SCOPE_PUBLIC};
static int addressScope(const struct sockaddr* addr){
    if(addr->sa_family==AF_INET){uint32_t a=ntohl(((const struct sockaddr_in*)addr)->sin_addr.s_addr);
        if((a>>24)==127)return SCOPE_LOOPBACK;if((a>>16)==0xA9FE)return SCOPE_LINKLOCAL;
        if((a>>24)==10||(a>>20)==0xAC1||(a>>16)==0xC0A8)return SCOPE_PRIVATE;if((a>>22)==(0x64400000u>>22))return SCOPE_CGNAT;return SCOPE_PUBLIC;}
    const struct in6_addr* v6=&((const struct sockaddr_in6*)addr)->sin6_addr;const uint8_t* b=v6->s6_addr;
    if(IN6_IS_ADDR_LOOPBACK(v6))return SCOPE_LOOPBACK;
    if(b[0]==0xfe&&(b[1]&0xc0)==0x80)return SCOPE_LINKLOCAL;if((b[0]&0xfe)==0xfc)return SCOPE_PRIVATE;return SCOPE_PUBLIC;
}

OBF static jstring integrity(JNIEnv* env,jclass){
    if(env->EnsureLocalCapacity(256)<0)return nullptr;
    jobject json=construct(env,"org/json/JSONObject","()V");NEED(json);
    jobject errors=construct(env,"org/json/JSONObject","()V");NEED(errors);
    jobject hooks=construct(env,"org/json/JSONArray","()V");NEED(hooks);
    jobject threads=construct(env,"org/json/JSONArray","()V");NEED(threads);
    jobject emulator=construct(env,"org/json/JSONArray","()V");NEED(emulator);
    jobject injected=construct(env,"org/json/JSONArray","()V");NEED(injected);

    FILE* file=fopen("/proc/self/status","r");
    if(file){char line[256];bool found=false;while(fgets(line,sizeof(line),file)){long pid;if(sscanf(line,"TracerPid: %ld",&pid)==1){jsonLong(env,json,"tracerPid",pid);found=true;break;}}fclose(file);if(!found)jsonText(env,errors,"tracerPid","TracerPid missing");}
    else jsonText(env,errors,"tracerPid",strerror(errno));

    file=fopen("/proc/self/maps","r");
    if(file){
        const size_t kinds=sizeof(HOOK_MAPS)/sizeof(HOOK_MAPS[0]);const char* reported[sizeof(HOOK_MAPS)/sizeof(HOOK_MAPS[0])];size_t count=0;char line[1024];
        bool injectSeen[4]={false,false,false,false};
        while(fgets(line,sizeof(line),file)){const char* path=strchr(line,'/');if(!path)path=strchr(line,'[');if(!path)continue;
            int inject=injectedKind(line,path);
            if(inject!=INJECT_NONE&&!injectSeen[inject]){injectSeen[inject]=true;char kind[24];
                switch(inject){case INJECT_TMP:strcpy(kind,"tmp_library");break;case INJECT_MEMFD:strcpy(kind,"memfd_code");break;default:strcpy(kind,"deleted_library");}
                jsonArrayPut(env,injected,kind);}
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

    jobject boot=construct(env,"org/json/JSONObject","()V");NEED(boot);
    jobject rootProps=construct(env,"org/json/JSONArray","()V");NEED(rootProps);
    PropScan scan={env,boot,rootProps,0,0};propReader=(PropReadCallback)dlsym(RTLD_DEFAULT,"__system_property_read_callback");
    if(__system_property_foreach(propVisit,&scan)<0)jsonText(env,errors,"properties","property area unavailable");
    NEED(boot);if(env->ExceptionCheck())return nullptr;

    jobject mounts=construct(env,"org/json/JSONArray","()V");NEED(mounts);
    file=fopen("/proc/self/mountinfo","r");
    if(file){const char* reported[sizeof(ROOT_MOUNTS)/sizeof(ROOT_MOUNTS[0])];size_t count=0;char line[1024];
        while(fgets(line,sizeof(line),file))for(const char* marker:ROOT_MOUNTS){if(!containsFold(line,marker))continue;
            bool duplicate=false;for(size_t j=0;j<count;j++)if(reported[j]==marker)duplicate=true;
            if(!duplicate){reported[count++]=marker;jsonArrayPut(env,mounts,marker);}}
        fclose(file);
    } else jsonText(env,errors,"mountinfo",strerror(errno));

    jobject interfaces=construct(env,"org/json/JSONArray","()V");NEED(interfaces);
    struct ifaddrs* list=nullptr;
    uint32_t ownAddresses[8];int ownCount=0;
    if(getifaddrs(&list)==0){char seen[32][48];size_t seenCount=0;
        for(struct ifaddrs* it=list;it;it=it->ifa_next){
            if(!it->ifa_addr||!it->ifa_name||!(it->ifa_flags&IFF_UP))continue;
            if(it->ifa_addr->sa_family!=AF_INET&&it->ifa_addr->sa_family!=AF_INET6)continue;
            int code=addressScope(it->ifa_addr);if(code==SCOPE_LOOPBACK)continue;
            if(it->ifa_addr->sa_family==AF_INET&&code!=SCOPE_LINKLOCAL&&ownCount<8)ownAddresses[ownCount++]=((const struct sockaddr_in*)it->ifa_addr)->sin_addr.s_addr;
            char scope[16];
            switch(code){case SCOPE_LINKLOCAL:strcpy(scope,"linklocal");break;case SCOPE_PRIVATE:strcpy(scope,"private");break;
                case SCOPE_CGNAT:strcpy(scope,"cgnat");break;default:strcpy(scope,"public");}
            char key[48];snprintf(key,sizeof(key),"%s|%d|%d",it->ifa_name,it->ifa_addr->sa_family,code);
            bool duplicate=false;for(size_t j=0;j<seenCount;j++)if(strcmp(seen[j],key)==0)duplicate=true;
            if(duplicate||seenCount==32)continue;strcpy(seen[seenCount++],key);
            jobject item=construct(env,"org/json/JSONObject","()V");NEED(item);
            jsonText(env,item,"name",it->ifa_name);jsonText(env,item,"family",it->ifa_addr->sa_family==AF_INET?"ipv4":"ipv6");jsonText(env,item,"scope",scope);
            jobject result=call(env,interfaces,"put","(Ljava/lang/Object;)Lorg/json/JSONArray;",item);
            if(result)env->DeleteLocalRef(result);env->DeleteLocalRef(item);if(env->ExceptionCheck()){freeifaddrs(list);return nullptr;}
        }
        freeifaddrs(list);
    } else jsonText(env,errors,"interfaces",strerror(errno));

    // Local listeners: Frida answers its own handshake; a listener also reachable on the
    // device's own network address is exposed to the LAN (remote control, debug bridges).
    jobject ports=construct(env,"org/json/JSONObject","()V");NEED(ports);
    jobject open=construct(env,"org/json/JSONArray","()V");NEED(open);
    jobject frida=construct(env,"org/json/JSONArray","()V");NEED(frida);
    jobject exposed=construct(env,"org/json/JSONArray","()V");NEED(exposed);
    struct timespec scanStart;clock_gettime(CLOCK_MONOTONIC,&scanStart);
    uint16_t listening[64];int failure=0;int total=scanLoopback(listening,64,&failure);
    if(failure)jsonText(env,errors,"ports",strerror(failure));
    long scanMillis=elapsedMillis(&scanStart);
    int probed=total<PROBE_MAX?total:PROBE_MAX;bool isFrida[PROBE_MAX];
    fridaListeners(listening,probed,isFrida);
    for(int i=0;i<total&&i<64;i++){
        if(!jsonArrayLong(env,open,listening[i]))return nullptr;
        if(i<probed&&isFrida[i]&&!jsonArrayLong(env,frida,listening[i]))return nullptr;
        for(int a=0;a<ownCount;a++){int fd=connectTimed(ownAddresses[a],listening[i],100);
            if(fd>=0){close(fd);if(!jsonArrayLong(env,exposed,listening[i]))return nullptr;break;}}
    }
    jsonLong(env,ports,"count",total);jsonLong(env,ports,"scanMillis",scanMillis);jsonLong(env,ports,"probeMillis",elapsedMillis(&scanStart)-scanMillis);
    call(env,ports,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("loopback"),open);
    call(env,ports,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("frida"),frida);
    call(env,ports,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("exposed"),exposed);
    if(env->ExceptionCheck())return nullptr;

    file=fopen("/proc/self/attr/current","r");
    if(file){char context[256]={0};if(fgets(context,sizeof(context),file)){context[strcspn(context,"\r\n")]=0;jsonText(env,json,"selinuxContext",context);}fclose(file);}
    else jsonText(env,errors,"selinuxContext",strerror(errno));
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("bootState"),boot);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("rootProperties"),rootProps);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("rootMounts"),mounts);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("interfaces"),interfaces);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("hooks"),hooks);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("hookThreads"),threads);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("emulator"),emulator);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("injectedCode"),injected);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("ports"),ports);
    call(env,json,"put","(Ljava/lang/String;Ljava/lang/Object;)Lorg/json/JSONObject;",env->NewStringUTF("errors"),errors);
    return (jstring)call(env,json,"toString","()Ljava/lang/String;");
}
#else
static jstring integrity(JNIEnv* env,jclass){error(env,"native integrity collection requires Android");return nullptr;}
#endif
