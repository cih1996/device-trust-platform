#include <jni.h>
#include <stdint.h>
#include <string.h>
#include <stdlib.h>
#include <stdarg.h>

#define OBF __attribute__((annotate("obf: strenc, flattening(minBlocks=3), bcf(prob=15), substitution(loop=1), shield")))
#define NEED(x) if (!(x) || env->ExceptionCheck()) return nullptr

static jobject instance(JNIEnv* env,const char* cls,const char* algorithm) {
    jclass c=env->FindClass(cls); if(!c) return nullptr;
    char signature[160];
    const char* prefix="(Ljava/lang/String;)L";
    size_t n=strlen(prefix),m=strlen(cls);
    memcpy(signature,prefix,n); memcpy(signature+n,cls,m); signature[n+m]=';'; signature[n+m+1]=0;
    jmethodID method=env->GetStaticMethodID(c,"getInstance",signature);if(!method)return nullptr;
    return env->CallStaticObjectMethod(c,method,env->NewStringUTF(algorithm));
}
static jobject call(JNIEnv* env,jobject target,const char* name,const char* signature,...) {
    if(!target||env->ExceptionCheck())return nullptr;
    jclass cls=env->GetObjectClass(target);if(!cls)return nullptr;
    jmethodID method=env->GetMethodID(cls,name,signature);if(!method)return nullptr;
    va_list args;va_start(args,signature);jobject result=env->CallObjectMethodV(target,method,args);va_end(args);return result;
}
static bool run(JNIEnv* env,jobject target,const char* name,const char* signature,...) {
    if(!target||env->ExceptionCheck())return false;
    jclass cls=env->GetObjectClass(target);if(!cls)return false;
    jmethodID method=env->GetMethodID(cls,name,signature);if(!method)return false;
    va_list args;va_start(args,signature);env->CallVoidMethodV(target,method,args);va_end(args);return !env->ExceptionCheck();
}
static jobject construct(JNIEnv* env,const char* name,const char* signature,...) {
    if(env->ExceptionCheck())return nullptr;
    jclass cls=env->FindClass(name);if(!cls)return nullptr;
    jmethodID method=env->GetMethodID(cls,"<init>",signature);if(!method)return nullptr;
    va_list args;va_start(args,signature);jobject result=env->NewObjectV(cls,method,args);va_end(args);return result;
}
static jbyteArray array(JNIEnv* env,const void* data,int size) {
    jbyteArray result=env->NewByteArray(size);if(result&&size)env->SetByteArrayRegion(result,0,size,(const jbyte*)data);return result;
}
static jobject keySpec(JNIEnv* env,jbyteArray key,const char* algorithm) {
    return construct(env,"javax/crypto/spec/SecretKeySpec","([BLjava/lang/String;)V",key,env->NewStringUTF(algorithm));
}
static void error(JNIEnv* env,const char* text) {if(!env->ExceptionCheck())env->ThrowNew(env->FindClass("java/security/GeneralSecurityException"),text);}

// Cryptographic primitives are supplied by JCA/Android Keystore. The private
// signing key stays in its provider; no key bytes are exported into this library.
OBF static jbyteArray protect(JNIEnv* env,jclass,jint kind,jbyteArray challenge,jbyteArray keyId,jbyteArray salt,jbyteArray server,jbyteArray payload,jobject signingKey) {
    if(env->EnsureLocalCapacity(256)<0)return nullptr;
    if(!challenge||!keyId||!salt||!server||!payload||!signingKey){error(env,"null frame parameter");return nullptr;}
    int size=env->GetArrayLength(payload);
    if((kind!=1&&kind!=2)||env->GetArrayLength(challenge)!=24||env->GetArrayLength(keyId)!=32||env->GetArrayLength(salt)!=32||size>250000){error(env,"invalid binary frame parameters");return nullptr;}
    jobject generator=instance(env,"java/security/KeyPairGenerator","EC");NEED(generator);
    jobject curve=construct(env,"java/security/spec/ECGenParameterSpec","(Ljava/lang/String;)V",env->NewStringUTF("secp256r1"));NEED(curve);
    if(!run(env,generator,"initialize","(Ljava/security/spec/AlgorithmParameterSpec;)V",curve))return nullptr;
    jobject pair=call(env,generator,"generateKeyPair","()Ljava/security/KeyPair;");NEED(pair);
    jobject privateKey=call(env,pair,"getPrivate","()Ljava/security/PrivateKey;");NEED(privateKey);
    jobject publicKey=call(env,pair,"getPublic","()Ljava/security/PublicKey;");NEED(publicKey);
    jbyteArray encoded=(jbyteArray)call(env,publicKey,"getEncoded","()[B");NEED(encoded);
    if(env->GetArrayLength(encoded)!=91){error(env,"unexpected P-256 public encoding");return nullptr;}
    jobject factory=instance(env,"java/security/KeyFactory","EC");NEED(factory);
    jobject publicSpec=construct(env,"java/security/spec/X509EncodedKeySpec","([B)V",server);NEED(publicSpec);
    jobject serverKey=call(env,factory,"generatePublic","(Ljava/security/spec/KeySpec;)Ljava/security/PublicKey;",publicSpec);NEED(serverKey);
    jobject agreement=instance(env,"javax/crypto/KeyAgreement","ECDH");NEED(agreement);
    if(!run(env,agreement,"init","(Ljava/security/Key;)V",privateKey))return nullptr;
    call(env,agreement,"doPhase","(Ljava/security/Key;Z)Ljava/security/Key;",serverKey,JNI_TRUE);if(env->ExceptionCheck())return nullptr;
    jbyteArray secret=(jbyteArray)call(env,agreement,"generateSecret","()[B");NEED(secret);
    jobject mac=instance(env,"javax/crypto/Mac","HmacSHA256");NEED(mac);
    jobject saltKey=keySpec(env,salt,"HmacSHA256");NEED(saltKey);
    if(!run(env,mac,"init","(Ljava/security/Key;)V",saltKey))return nullptr;
    jbyteArray prk=(jbyteArray)call(env,mac,"doFinal","([B)[B",secret);NEED(prk);
    jobject extractKey=keySpec(env,prk,"HmacSHA256");NEED(extractKey);
    if(!run(env,mac,"init","(Ljava/security/Key;)V",extractKey))return nullptr;
    const unsigned char info[]="device-trust/frame/v1\1";
    jbyteArray expandInput=array(env,info,sizeof(info)-1);NEED(expandInput);
    jbyteArray aesBytes=(jbyteArray)call(env,mac,"doFinal","([B)[B",expandInput);NEED(aesBytes);
    jobject aesKey=keySpec(env,aesBytes,"AES");NEED(aesKey);
    unsigned char zero[32]={0};
    env->SetByteArrayRegion(secret,0,32,(jbyte*)zero);env->SetByteArrayRegion(prk,0,32,(jbyte*)zero);env->SetByteArrayRegion(aesBytes,0,32,(jbyte*)zero);
    jbyteArray nonce=env->NewByteArray(12);NEED(nonce);
    jobject random=construct(env,"java/security/SecureRandom","()V");NEED(random);
    if(!run(env,random,"nextBytes","([B)V",nonce))return nullptr;
    unsigned char header[142]={ 'D','T','P','4' };header[4]=(unsigned char)kind;
    env->GetByteArrayRegion(challenge,0,24,(jbyte*)(header+5));env->GetByteArrayRegion(keyId,0,32,(jbyte*)(header+29));
    env->GetByteArrayRegion(encoded,26,65,(jbyte*)(header+61));env->GetByteArrayRegion(nonce,0,12,(jbyte*)(header+126));
    uint32_t encryptedSize=(uint32_t)size+16;
    for(int i=0;i<4;i++)header[138+i]=(unsigned char)(encryptedSize>>(24-i*8));
    jobject cipher=instance(env,"javax/crypto/Cipher","AES/GCM/NoPadding");NEED(cipher);
    jobject gcm=construct(env,"javax/crypto/spec/GCMParameterSpec","(I[B)V",128,nonce);NEED(gcm);
    if(!run(env,cipher,"init","(ILjava/security/Key;Ljava/security/spec/AlgorithmParameterSpec;)V",1,aesKey,gcm))return nullptr;
    jbyteArray aad=array(env,header,142);NEED(aad);
    if(!run(env,cipher,"updateAAD","([B)V",aad))return nullptr;
    jbyteArray encrypted=(jbyteArray)call(env,cipher,"doFinal","([B)[B",payload);NEED(encrypted);
    int encryptedLength=env->GetArrayLength(encrypted);
    if(encryptedLength!=(int)encryptedSize){error(env,"unexpected GCM payload size");return nullptr;}
    int signedLength=142+encryptedLength;
    jbyteArray signedData=env->NewByteArray(signedLength);NEED(signedData);
    env->SetByteArrayRegion(signedData,0,142,(jbyte*)header);
    jbyte* encryptedData=env->GetByteArrayElements(encrypted,nullptr);NEED(encryptedData);
    env->SetByteArrayRegion(signedData,142,encryptedLength,encryptedData);env->ReleaseByteArrayElements(encrypted,encryptedData,JNI_ABORT);
    jobject signer=instance(env,"java/security/Signature","SHA256withECDSA");NEED(signer);
    if(!run(env,signer,"initSign","(Ljava/security/PrivateKey;)V",signingKey))return nullptr;
    if(!run(env,signer,"update","([B)V",signedData))return nullptr;
    jbyteArray signature=(jbyteArray)call(env,signer,"sign","()[B");NEED(signature);
    int sigLength=env->GetArrayLength(signature);
    if(sigLength<8||sigLength>80){error(env,"unexpected ECDSA signature size");return nullptr;}
    jbyteArray output=env->NewByteArray(signedLength+2+sigLength);NEED(output);
    jbyte* signedBytes=env->GetByteArrayElements(signedData,nullptr);NEED(signedBytes);
    env->SetByteArrayRegion(output,0,signedLength,signedBytes);env->ReleaseByteArrayElements(signedData,signedBytes,JNI_ABORT);
    jbyte lengthBytes[2]={(jbyte)(sigLength>>8),(jbyte)sigLength};env->SetByteArrayRegion(output,signedLength,2,lengthBytes);
    jbyte* signatureBytes=env->GetByteArrayElements(signature,nullptr);NEED(signatureBytes);
    env->SetByteArrayRegion(output,signedLength+2,sigLength,signatureBytes);env->ReleaseByteArrayElements(signature,signatureBytes,JNI_ABORT);
    return output;
}

OBF static jboolean pinMatches(JNIEnv* env,jclass,jbyteArray digest,jbyteArray expected) {
    if(!digest||!expected||env->GetArrayLength(digest)!=32||env->GetArrayLength(expected)!=32)return JNI_FALSE;
    unsigned char a[32],b[32];env->GetByteArrayRegion(digest,0,32,(jbyte*)a);env->GetByteArrayRegion(expected,0,32,(jbyte*)b);
    if(env->ExceptionCheck())return JNI_FALSE;
    unsigned int difference=0;for(int i=0;i<32;i++)difference|=(unsigned int)(a[i]^b[i]);
    return difference==0?JNI_TRUE:JNI_FALSE;
}

extern "C" JNIEXPORT jint JNICALL JNI_OnLoad(JavaVM* vm,void*) {
    JNIEnv* env=nullptr;if(vm->GetEnv((void**)&env,JNI_VERSION_1_6)!=JNI_OK)return JNI_ERR;
    jclass cls=env->FindClass("com/device/trust/secure/NativeFrame");if(!cls)return JNI_ERR;
    JNINativeMethod methods[]={
        {(char*)"protect",(char*)"(I[B[B[B[B[BLjava/security/PrivateKey;)[B",(void*)protect},
        {(char*)"pinMatches",(char*)"([B[B)Z",(void*)pinMatches}
    };
    return env->RegisterNatives(cls,methods,2)==0?JNI_VERSION_1_6:JNI_ERR;
}
