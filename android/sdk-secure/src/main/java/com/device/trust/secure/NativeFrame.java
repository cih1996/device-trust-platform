package com.device.trust.secure;

import java.security.PrivateKey;

public final class NativeFrame {
    static { System.loadLibrary("dtpsecure"); }
    private NativeFrame() {}
    public static native byte[] protect(int kind,byte[] challengeId,byte[] keyId,byte[] salt,byte[] serverPublicKey,byte[] payload,PrivateKey key) throws java.security.GeneralSecurityException;
    public static native boolean pinMatches(byte[] digest,byte[] expected);
    public static native String resources() throws java.security.GeneralSecurityException;
}
