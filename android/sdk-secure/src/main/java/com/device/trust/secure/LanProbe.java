package com.device.trust.secure;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

/**
 * Probes the LAN addresses the server lists in challenge.lanTargets: other SDK devices it
 * last saw on this device's LAN. Only addresses inside this device's own Wi-Fi/Ethernet
 * subnet are touched; nothing else on the network is scanned.
 */
final class LanProbe {
    private LanProbe() {}
    static final int ADB_PORT = 5555;
    private static final int TIMEOUT_MS = 300, BUDGET_MS = 3000, THREADS = 16;

    static void attach(JSONObject challenge, JSONObject report) throws JSONException {
        JSONArray requested = challenge.optJSONArray("lanTargets");
        if (requested == null || requested.length() == 0) return;
        JSONObject signals = report.getJSONObject("signals");
        JSONObject lan = signals.optJSONObject("lan");
        if (lan == null || !"available".equals(lan.optString("status"))) {
            signals.put("lanProbe", new JSONObject().put("status", "unavailable").put("reason", "no LAN signal"));
            return;
        }
        JSONObject value = lan.getJSONObject("value");
        JSONArray own = value.getJSONArray("addresses");
        List<Integer> targets = new ArrayList<>();
        for (int i = 0; i < requested.length(); i++) {
            int ip = ipv4(requested.optString(i));
            if (ip != 0 && inOwnSubnet(own, ip) && !targets.contains(ip)) targets.add(ip);
        }
        int gateway = ipv4(value.optString("gateway", ""));
        long started = System.nanoTime();
        ExecutorService pool = Executors.newFixedThreadPool(THREADS);
        try {
            List<Callable<boolean[]>> tasks = new ArrayList<>();
            for (int ip : targets) tasks.add(() -> new boolean[]{reachable(ip), open(ip, ADB_PORT)});
            if (gateway != 0) tasks.add(() -> new boolean[]{reachable(gateway), false});
            List<Future<boolean[]>> results = pool.invokeAll(tasks, BUDGET_MS, TimeUnit.MILLISECONDS);
            JSONArray reachable = new JSONArray(), adb = new JSONArray();
            for (int i = 0; i < targets.size(); i++) {
                boolean[] r = done(results.get(i));
                if (r[0]) reachable.put(text(targets.get(i)));
                if (r[1]) adb.put(text(targets.get(i)));
            }
            JSONObject probe = new JSONObject().put("targets", targets.size()).put("reachable", reachable).put("adb", adb)
                    .put("millis", (System.nanoTime() - started) / 1000000);
            if (gateway != 0) probe.put("gateway", done(results.get(targets.size()))[0]);
            signals.put("lanProbe", new JSONObject().put("status", "available").put("value", probe));
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            signals.put("lanProbe", new JSONObject().put("status", "error").put("reason", "interrupted"));
        } finally {
            pool.shutdownNow();
        }
    }

    // A task cut off by the time budget counts as no answer.
    private static boolean[] done(Future<boolean[]> f) {
        try { return f.isCancelled() ? new boolean[2] : f.get(); } catch (Exception e) { return new boolean[2]; }
    }
    // ICMP echo through the unprivileged ping socket, else a TCP connect to port 7.
    private static boolean reachable(int ip) {
        try { return address(ip).isReachable(TIMEOUT_MS); } catch (Exception e) { return false; }
    }
    private static boolean open(int ip, int port) {
        try (Socket s = new Socket()) { s.connect(new InetSocketAddress(address(ip), port), TIMEOUT_MS); return true; }
        catch (Exception e) { return false; }
    }
    private static boolean inOwnSubnet(JSONArray own, int ip) {
        for (int i = 0; i < own.length(); i++) {
            JSONObject a = own.optJSONObject(i);
            if (a == null) continue;
            int self = ipv4(a.optString("address")), prefix = a.optInt("prefix", 0);
            if (self == 0 || prefix < 8 || prefix > 30 || self == ip) continue;
            int mask = -1 << (32 - prefix);
            if ((self & mask) == (ip & mask)) return true;
        }
        return false;
    }
    // Dotted-quad literal only, so nothing ever goes to DNS. 0 means invalid.
    static int ipv4(String text) {
        String[] parts = text.split("\\.", -1);
        if (parts.length != 4) return 0;
        int ip = 0;
        for (String p : parts) {
            if (p.isEmpty() || p.length() > 3) return 0;
            for (int i = 0; i < p.length(); i++) if (!Character.isDigit(p.charAt(i))) return 0;
            int b = Integer.parseInt(p);
            if (b > 255) return 0;
            ip = (ip << 8) | b;
        }
        return ip;
    }
    private static String text(int ip) { return (ip >>> 24) + "." + ((ip >>> 16) & 255) + "." + ((ip >>> 8) & 255) + "." + (ip & 255); }
    private static InetAddress address(int ip) throws Exception {
        return InetAddress.getByAddress(new byte[]{(byte) (ip >>> 24), (byte) (ip >>> 16), (byte) (ip >>> 8), (byte) ip});
    }
}
