package platform

import "strings"

// Tooling rules score traces of tools that drive, inspect or alter the app:
// installed risk apps, accessibility automation, injected input, Frida and
// other local listeners, and code injected into the process.

// riskAppCategories maps the packages the SDK looks up to a risk category.
// The SDK can only see names it declares in its <queries>.
var riskAppCategories = map[string]string{
	"com.topjohnwu.magisk": "root", "io.github.vvb2060.magisk": "root", "io.github.huskydg.magisk": "root",
	"me.weishu.kernelsu": "root", "com.rifsxd.ksunext": "root", "me.bmax.apatch": "root",
	"org.lsposed.manager": "hook", "de.robv.android.xposed.installer": "hook", "org.meowcat.edxposed.manager": "hook",
	"com.solohsu.android.edxp.manager": "hook", "top.canyie.dreamland.manager": "hook", "com.tsng.hidemyapplist": "hook",
	"moe.shizuku.privileged.api": "privilege",
	"io.va.exposed":              "virtual", "com.lbe.parallel.intl": "virtual", "com.excelliance.dualaid": "virtual",
	"com.bly.dkplat": "virtual", "com.vmos.pro": "virtual", "com.vphonegaga.titan": "virtual", "com.x8zs.sandbox": "virtual",
	"org.autojs.autojs": "automation", "org.autojs.autojspro": "automation", "org.autojs.autoxjs.v6": "automation",
	"com.stardust.scriptdroid": "automation", "com.cyjh.mobileanjian": "automation", "com.touchsprite.android": "automation",
	"net.dinglisch.android.taskerm": "automation", "com.llamalab.automate": "automation",
	"com.lexa.fakegps": "mock_location", "com.incorporateapps.fakegps.fre": "mock_location",
	"com.blogspot.newapphorizons.fakegps": "mock_location", "com.theappninjas.fakegpsjoystick": "mock_location",
	"com.guoshi.httpcanary": "capture", "com.guoshi.httpcanary.premium": "capture", "app.greyshirts.sslcapture": "capture",
	"com.minhui.networkcapture": "capture", "com.reqable.android": "capture", "com.emanuelef.remote_capture": "capture",
	"catch_.me_.if_.you_.can_":           "cheat",
	"com.teamviewer.quicksupport.market": "remote_control", "com.anydesk.anydeskandroid": "remote_control", "com.oray.sunlogin": "remote_control",
}

// riskAppPoints is scored once per category, in this order.
var riskAppPoints = []struct {
	category string
	points   int
}{{"hook", 35}, {"root", 30}, {"virtual", 25}, {"automation", 25}, {"cheat", 25}, {"mock_location", 20}, {"capture", 20}, {"privilege", 10}, {"remote_control", 10}}

// Debug servers with fixed default ports: Frida, IDA android_server.
const (
	fridaDefaultPort = 27042
	idaDefaultPort   = 23946
)

func toolingSignals(r Report, add func(int, string)) {
	if apps := signalMap(r, "riskApps"); apps != nil {
		found := map[string]bool{}
		installed, _ := apps["installed"].([]any)
		for _, raw := range installed {
			name, _ := raw.(string)
			if category := riskAppCategories[name]; category != "" {
				found[category] = true
			}
		}
		for _, c := range riskAppPoints {
			if found[c.category] {
				add(c.points, "risk_app_"+c.category)
			}
		}
	}

	if a := signalMap(r, "accessibility"); a != nil {
		services, _ := a["services"].([]any)
		configured, _ := a["configured"].([]any)
		system := map[string]bool{}
		automation := false
		for _, raw := range services {
			s, _ := raw.(map[string]any)
			name, _ := s["package"].(string)
			if s["system"] == true {
				system[name] = true
			} else if s["gestures"] == true {
				automation = true
			}
		}
		// A configured service the manager did not return is hidden by package
		// visibility; system packages are visible, so it is a third-party app.
		for _, raw := range configured {
			if name, _ := raw.(string); name != "" && !system[name] && !listed(services, name) {
				automation = true
			}
		}
		if automation {
			add(20, "accessibility_automation")
		}
		// Accessibility on with no service anywhere: a UiAutomation client
		// (UiAutomator, Appium) or a service registered outside Settings.
		if a["enabled"] == true && len(services) == 0 && len(configured) == 0 {
			add(25, "hidden_accessibility_client")
		}
	}

	if auto := signalMap(r, "automation"); auto != nil {
		if auto["monkey"] == true {
			add(30, "monkey_test")
		}
		touches, _ := auto["touches"].(map[string]any)
		downs, _ := touches["downs"].(float64)
		synthetic, _ := touches["synthetic"].(float64)
		if synthetic >= 2 || (synthetic >= 1 && synthetic == downs) {
			add(30, "injected_touches")
		}
	}

	if hooks := signalMap(r, "javaHooks"); hooks != nil {
		classes, _ := hooks["classes"].([]any)
		frames, _ := hooks["frames"].([]any)
		if len(classes) > 0 || len(frames) > 0 {
			add(40, "java_hook_framework")
		}
	}

	native := signalMap(r, "integrityNative")
	if injected, _ := native["injectedCode"].([]any); len(injected) > 0 {
		add(35, "injected_code")
	}
	ports, _ := native["ports"].(map[string]any)
	if ports == nil {
		return
	}
	loopback, _ := ports["loopback"].([]any)
	frida, _ := ports["frida"].([]any)
	exposed, _ := ports["exposed"].([]any)
	switch {
	case len(frida) > 0:
		add(50, "frida_server")
	case hasPort(loopback, fridaDefaultPort):
		add(30, "frida_default_port")
	}
	if hasPort(loopback, idaDefaultPort) {
		add(30, "debug_server_port")
	}
	if len(exposed) > 0 {
		add(15, "exposed_listening_ports")
	}
}

func listed(services []any, name string) bool {
	for _, raw := range services {
		if s, _ := raw.(map[string]any); s["package"] == name {
			return true
		}
	}
	return false
}

func hasPort(ports []any, port int) bool {
	for _, raw := range ports {
		if p, _ := raw.(float64); int(p) == port {
			return true
		}
	}
	return false
}

// vendorFamilies groups brands with the system packages every Chinese-market
// build of that family preinstalls. Honor phones made before the split still
// carry Huawei ID, so it counts for both.
var vendorFamilies = []struct {
	family   string
	brands   []string
	packages []string
}{
	{"xiaomi", []string{"xiaomi", "redmi", "poco"}, []string{"com.miui.securitycenter", "com.miui.home", "com.xiaomi.market"}},
	{"huawei", []string{"huawei"}, []string{"com.huawei.hwid", "com.huawei.appmarket"}},
	{"honor", []string{"honor"}, []string{"com.hihonor.appmarket", "com.hihonor.id", "com.huawei.hwid"}},
	{"oplus", []string{"oppo", "oneplus", "realme", "oplus"}, []string{"com.heytap.market", "com.oplus.safecenter", "com.coloros.safecenter"}},
	{"vivo", []string{"vivo", "iqoo"}, []string{"com.bbk.appstore", "com.bbk.launcher2"}},
}

func vendorFamily(brand string) int {
	brand = strings.ToLower(strings.TrimSpace(brand))
	for i, f := range vendorFamilies {
		for _, b := range f.brands {
			if brand == b {
				return i
			}
		}
	}
	return -1
}

// vendorPackageChecks compares the claimed brand with the vendor system apps
// present. A model-spoofing tool rewrites Build fields, not the system image.
// Only applied to devices on a Chinese SIM or IP: overseas builds of the same
// brands ship different system apps.
func vendorPackageChecks(r Report, java hardwareObservation, chinese bool, add func(code, state, dimension, detail, source string, points int)) {
	claimed := vendorFamily(java.Brand)
	if claimed < 0 {
		claimed = vendorFamily(java.Manufacturer)
	}
	var packages []packageObservation
	if claimed < 0 || !chinese || signalObject(r, "systemPackages", &packages) != nil {
		return
	}
	present := map[string]bool{}
	for _, p := range packages {
		if p.Status == "visible" && p.System {
			present[p.Package] = true
		}
	}
	own := vendorFamilies[claimed]
	found := false
	for _, name := range own.packages {
		found = found || present[name]
	}
	if !found {
		add("vendor_packages_missing", "conflict", "software", "品牌为 "+java.Brand+"，但没有该厂商的系统应用", "client", 25)
	} else {
		add("vendor_packages", "matched", "software", "厂商系统应用与品牌一致", "client", 0)
	}
	for i, f := range vendorFamilies {
		if i == claimed {
			continue
		}
		for _, name := range f.packages {
			if present[name] && !contains(own.packages, name) {
				add("foreign_vendor_packages", "conflict", "software", "品牌为 "+java.Brand+"，却有 "+name, "client", 25)
				return
			}
		}
	}
}

func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
