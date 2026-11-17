package platform

import "strings"

// Consistency rules compare independent observations of the same fact. A tool
// that rewrites one copy of a value rarely rewrites every copy, so a
// contradiction is stronger evidence than any single reading.

// chinaCarrier groups mainland MCC/MNC codes by operator. Unknown codes return "".
func chinaCarrier(plmn string) string {
	switch plmn {
	case "46000", "46002", "46004", "46007", "46008", "46013":
		return "移动"
	case "46001", "46006", "46009", "46010":
		return "联通"
	case "46003", "46005", "46011", "46012":
		return "电信"
	case "46015":
		return "广电"
	}
	return ""
}

// ispCarrier maps an ip2region ISP name onto the same operator groups.
func ispCarrier(isp string) string {
	for _, name := range []string{"移动", "联通", "电信", "广电"} {
		if strings.Contains(isp, name) {
			return name
		}
	}
	return ""
}

// sharedNetwork reports whether a SIM may legitimately register on another
// operator's network without roaming: Telecom and Unicom share their RAN.
func sharedNetwork(sim, network string) bool {
	return sim == network || (sim == "电信" && network == "联通") || (sim == "联通" && network == "电信")
}

// carrierChecks compares the SIM issuer, the registered network and the
// public IP's operator. add has the ResourceAssessment check signature.
func carrierChecks(r Report, ipISP, ipCountry string, add func(code, state, dimension, detail, source string, points int)) {
	sim, network := stringSignal(r, "simOperator"), stringSignal(r, "operator")
	if sim == "" || network == "" || len(sim) < 5 || len(network) < 5 {
		add("sim_network", "unknown", "network", "SIM 或注册网络不可用", "client", 0)
	} else if signalTrue(r, "networkRoaming") {
		add("sim_network", "unknown", "network", "漫游中", "client", 0)
	} else if simGroup, netGroup := chinaCarrier(sim), chinaCarrier(network); simGroup != "" && netGroup != "" {
		if sharedNetwork(simGroup, netGroup) {
			add("sim_network", "matched", "network", "SIM "+simGroup+" / 网络 "+netGroup, "client", 0)
		} else {
			add("sim_network_mismatch", "conflict", "network", "SIM "+simGroup+"（"+sim+"）注册在 "+netGroup+"（"+network+"）且未漫游", "client", 25)
		}
	} else if sim[:3] != network[:3] {
		add("sim_network_mismatch", "conflict", "network", "SIM "+sim+" 与网络 "+network+" 国家码不同且未漫游", "client", 20)
	} else {
		add("sim_network", "matched", "network", "SIM "+sim+" / 网络 "+network, "client", 0)
	}

	if stringSignal(r, "transport") != "cellular" || signalTrue(r, "vpn") {
		return
	}
	simGroup, ipGroup := chinaCarrier(sim), ispCarrier(ipISP)
	switch {
	case ipCountry != "中国" || simGroup == "" || ipGroup == "":
		add("cellular_ip_carrier", "unknown", "network", "运营商或出口归属不可比较", "ip2region", 0)
	case simGroup == ipGroup:
		add("cellular_ip_carrier", "matched", "network", "SIM 与出口均为"+simGroup, "ip2region", 0)
	default:
		add("cellular_ip_carrier_mismatch", "conflict", "network", "移动数据但出口为"+ipISP+"，SIM 为"+simGroup, "ip2region", 20)
	}
}

// interfaceKind classifies kernel interface names by link type. "" means unknown.
func interfaceKind(name string) string {
	switch {
	case strings.Contains(name, "rmnet"), strings.HasPrefix(name, "ccmni"), strings.HasPrefix(name, "seth"),
		strings.HasPrefix(name, "pdp"), strings.HasPrefix(name, "wwan"), strings.HasPrefix(name, "ppp"):
		return "cellular"
	case strings.HasPrefix(name, "wlan"), strings.HasPrefix(name, "swlan"), strings.HasPrefix(name, "wifi"):
		return "wifi"
	case strings.HasPrefix(name, "eth"):
		return "ethernet"
	}
	return ""
}

// carrierInterface finds the link that carries routable traffic: the Java
// active interface, else the native interface with a non-link-local address.
func carrierInterface(r Report, native map[string]any) string {
	if name := stringSignal(r, "activeInterface"); name != "" {
		return name
	}
	items, _ := native["interfaces"].([]any)
	routable := ""
	for _, item := range items {
		entry, _ := item.(map[string]any)
		name, _ := entry["name"].(string)
		scope, _ := entry["scope"].(string)
		if scope == "linklocal" || interfaceKind(name) == "" {
			continue
		}
		if interfaceKind(name) == "cellular" {
			return "" // A routable cellular link exists; the claim may be true.
		}
		routable = name
	}
	return routable
}

var appDomains = []string{"untrusted_app", "isolated_app", "ephemeral_app", "sdk_sandbox", "platform_app", "priv_app", "system_app"}

// environmentConsistency scores contradictions inside one report.
func environmentConsistency(r Report, add func(int, string)) {
	native := signalMap(r, "integrityNative")
	if boot, _ := native["bootState"].(map[string]any); boot != nil {
		green, broken, unlocked := false, false, false
		for name, raw := range boot {
			value, _ := raw.(string)
			switch {
			case strings.HasSuffix(name, "verifiedbootstate") && value == "green":
				green = true
			case strings.HasSuffix(name, "verifiedbootstate") && value != "":
				broken = true
			case strings.HasSuffix(name, "flash.locked") && value == "0", value == "unlocked":
				unlocked = true
			}
		}
		if green && (broken || unlocked) {
			add(40, "boot_state_spoofed")
		} else if broken || unlocked {
			add(20, "bootloader_unlocked")
		}
	}
	if names, _ := native["rootProperties"].([]any); len(names) > 0 {
		add(35, "root_properties")
	}
	if mounts, _ := native["rootMounts"].([]any); len(mounts) > 0 {
		add(40, "root_mounts")
	}
	if context, _ := native["selinuxContext"].(string); context != "" {
		parts := strings.Split(context, ":")
		known := len(parts) >= 3
		if known {
			known = false
			for _, domain := range appDomains {
				known = known || strings.HasPrefix(parts[2], domain)
			}
		}
		if !known {
			add(30, "selinux_domain")
		}
	}
	if os := signalMap(r, "osInfo"); os != nil {
		display, _ := os["display"].(string)
		tags, _ := os["tags"].(string)
		if tags == "release-keys" && (strings.Contains(display, "dev-keys") || strings.Contains(display, "test-keys")) {
			add(25, "build_tags_mismatch")
		}
	}
	if claimed := stringSignal(r, "transport"); (claimed == "cellular" || claimed == "wifi") && !signalTrue(r, "vpn") {
		if kind := interfaceKind(carrierInterface(r, native)); kind != "" && kind != claimed {
			add(30, "network_interface_mismatch")
		}
	}
}

// hostingISPs are ip2region ISP names of cloud and hosting networks. A phone
// reaching the server from one is behind a proxy, VPN server or cloud phone.
var hostingISPs = []string{"阿里", "腾讯", "华为", "百度", "京东", "金山", "UCloud", "Amazon", "Google", "Microsoft", "Oracle", "DigitalOcean", "Vultr", "Linode", "Akamai", "OVH", "Hetzner", "Alibaba", "Tencent"}

func egressChecks(r Report, ip *IPRegion, add func(code, state, dimension, detail, source string, points int)) {
	if ip == nil {
		return
	}
	for _, name := range hostingISPs {
		if strings.Contains(ip.ISP, name) {
			add("datacenter_egress", "conflict", "network", "出口属于 "+ip.ISP+" 机房网络", "ip2region", 30)
			break
		}
	}
	sim := stringSignal(r, "simOperator")
	if !strings.HasPrefix(sim, "460") || signalTrue(r, "networkRoaming") {
		return
	}
	outside := ip.CountryCode != "CN"
	for _, region := range []string{"香港", "澳门", "台湾"} {
		outside = outside || strings.Contains(ip.Province, region)
	}
	if outside {
		add("ip_outside_sim_country", "conflict", "network", "中国大陆 SIM，出口在 "+ip.Country+" "+ip.Province, "ip2region", 20)
	}
}

// sameOperator reports whether two operator groups can serve each other's SIMs:
// Telecom/Unicom and Mobile/Broadnet share radio networks.
func sameOperator(a, b string) bool {
	return sharedNetwork(a, b) || (a == "移动" && b == "广电") || (a == "广电" && b == "移动")
}

// cellChecks compares the modem's registered cells with the operator and SIM
// the telephony framework reports. Operator-spoofing tools rewrite the
// framework values; the cell list comes from the radio.
func cellChecks(r Report, cells []cellObservation, add func(code, state, dimension, detail, source string, points int)) {
	sim, network := stringSignal(r, "simOperator"), stringSignal(r, "operator")
	roaming := signalTrue(r, "networkRoaming")
	for _, cell := range cells {
		if !cell.Registered || cell.MCC == "" || cell.MNC == "" || cell.Age == nil || *cell.Age < 0 || *cell.Age > 120000 {
			continue
		}
		plmn := cell.MCC + cell.MNC
		if len(network) >= 5 {
			a, b := chinaCarrier(plmn), chinaCarrier(network)
			if (a != "" && b != "" && !sameOperator(a, b)) || cell.MCC != network[:3] {
				add("cell_operator_mismatch", "conflict", "network", "注册基站 "+plmn+"，系统报告网络 "+network, "client", 25)
				return
			}
		}
		if len(sim) >= 3 && !roaming && cell.MCC != sim[:3] {
			add("cell_sim_country_mismatch", "conflict", "network", "注册基站国家码 "+cell.MCC+"，SIM "+sim, "client", 20)
			return
		}
		add("cell_operator", "matched", "network", "注册基站与运营商一致", "client", 0)
		return
	}
}
