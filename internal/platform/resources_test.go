package platform

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resourceReport() Report {
	return Report{ObservedIP: "127.0.0.1", Signals: map[string]Signal{
		"resourceJava":     {Status: "available", Value: map[string]any{"brand": "Xiaomi", "model": "MI 8 SE", "device": "sirius", "hardware": "qcom", "memoryBytes": float64(6 << 30), "availableCores": 8}},
		"resourceNative":   {Status: "available", Value: map[string]any{"brand": "Xiaomi", "model": "MI 8 SE", "device": "sirius", "hardware": "qcom", "platform": "sdm710", "memoryBytes": float64(6 << 30), "configuredCores": 8}},
		"buildFingerprint": {Status: "available", Value: "Xiaomi/sirius/sirius:10/test"},
		"systemPackages":   {Status: "available", Value: []map[string]any{{"package": "com.test.system", "status": "visible", "signers": []string{"approved"}, "system": true}}},
		"location":         {Status: "permission_denied", Reason: "test permission not granted"}, "cell": {Status: "unavailable", Reason: "test no SIM"},
	}}
}
func TestResourceMismatchAndMissingCoverage(t *testing.T) {
	engine := &ResourceEngine{Catalog: ResourceCatalog{Version: "TEST", Devices: []DeviceBaseline{{ID: "test-only", Brand: "Xiaomi", Model: "MI 8 SE", Device: "sirius", MemoryGiB: []float64{4, 6}, Cores: 8, Hardware: []string{"qcom"}, Platforms: []string{"sdm710"}, Source: "TEST ONLY", Packages: []PackageBaseline{{Package: "com.test.system", Required: true, Signers: []string{"approved"}}}}}}}
	r := resourceReport()
	assessment, err := engine.Assess(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range assessment.Checks {
		if check.Points > 0 {
			t.Fatal("clean fixture hit", check)
		}
	}
	r.Signals["resourceJava"].Value.(map[string]any)["memoryBytes"] = float64(16 << 30)
	r.Signals["systemPackages"] = Signal{Status: "available", Value: []map[string]any{{"package": "com.test.system", "status": "visible", "signers": []string{"forged"}}}}
	assessment, err = engine.Assess(r)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, check := range assessment.Checks {
		if check.Points > 0 {
			codes[check.Code] = true
		}
	}
	if !codes["memory_source_mismatch"] || !codes["system_package_signer_mismatch"] {
		t.Fatal(codes)
	}
	if codes["gps_coverage"] || codes["cell_coverage"] {
		t.Fatal("missing permissions scored as fraud")
	}
}
func TestCellGPSUsesAgeAccuracyAndKnownCell(t *testing.T) {
	now := time.Now().UTC()
	engine := &ResourceEngine{Catalog: ResourceCatalog{Version: "TEST"}, Cells: testCellDB(t, fmt.Sprintf("LTE,460,0,1,2,0,121,31,1000,5,1,1,%d,0", now.Unix()))}
	r := resourceReport()
	r.Signals["cell"] = Signal{Status: "available", Value: []map[string]any{{"technology": "LTE", "mcc": "460", "mnc": "00", "areaCode": 1, "cellId": 2, "registered": true, "ageMillis": 100}}}
	r.Signals["location"] = Signal{Status: "available", Value: map[string]any{"latitude": 39.9, "longitude": 116.4, "ageMillis": 100, "accuracyMeters": 20, "coordinateSystem": "WGS84"}}
	has := func(code string) bool {
		t.Helper()
		a, err := engine.Assess(r)
		if err != nil {
			t.Fatal(err)
		}
		for _, check := range a.Checks {
			if check.Code == code && check.Points > 0 {
				return true
			}
		}
		return false
	}
	if !has("cell_gps_mismatch") {
		t.Fatal("known distant cell not detected")
	}
	r.Signals["location"].Value.(map[string]any)["ageMillis"] = 300000
	if has("cell_gps_mismatch") {
		t.Fatal("stale GPS used for location conflict")
	}
}
func TestIPDatabaseSnapshots(t *testing.T) {
	dir := os.Getenv("DTP_RESOURCE_DIR")
	if dir == "" {
		t.Skip("optional public resource snapshot test")
	}
	for _, test := range []struct{ file, ip string }{{"ip2region_v4.xdb", "223.5.5.5"}, {"ip2region_v6.xdb", "2001:4860:4860::8888"}} {
		db, err := LoadIPDatabase(filepath.Join(dir, test.file))
		if err != nil {
			t.Fatal(err)
		}
		result, err := db.Lookup(netip.MustParseAddr(test.ip))
		if err != nil || result == nil {
			t.Fatal(result, err)
		}
		if len(result.CountryCode) != 2 {
			t.Fatal(result)
		}
	}
}
func TestRequestHashBindsMethodPathAccountAndBody(t *testing.T) {
	a := HashBusinessRequest("POST", "/pay", "u", []byte(`{"amount":100}`))
	for _, b := range []string{HashBusinessRequest("POST", "/pay", "u", []byte(`{"amount":101}`)), HashBusinessRequest("POST", "/pay", "v", []byte(`{"amount":100}`)), HashBusinessRequest("GET", "/pay", "u", []byte(`{"amount":100}`)), HashBusinessRequest("POST", "/claim", "u", []byte(`{"amount":100}`))} {
		if a == b {
			t.Fatal("request binding collision")
		}
	}
}
