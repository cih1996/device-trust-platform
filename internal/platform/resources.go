package platform

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"os"
	"strings"
	"time"
)

type ResourceCheck struct {
	Code      string `json:"code"`
	State     string `json:"state"`
	Dimension string `json:"dimension"`
	Detail    string `json:"detail,omitempty"`
	Source    string `json:"source,omitempty"`
	Points    int    `json:"points,omitempty"`
}
type ResourceAssessment struct {
	Version string          `json:"version"`
	IP      *IPRegion       `json:"ip,omitempty"`
	Checks  []ResourceCheck `json:"checks"`
}
type PackageBaseline struct {
	Package  string   `json:"package"`
	Signers  []string `json:"signers"`
	Required bool     `json:"required"`
}
type DeviceBaseline struct {
	ID             string            `json:"id"`
	Brand          string            `json:"brand"`
	Model          string            `json:"model"`
	Device         string            `json:"device"`
	Hardware       []string          `json:"hardware"`
	Platforms      []string          `json:"platforms"`
	MemoryGiB      []float64         `json:"memoryGiB"`
	Cores          int               `json:"cores"`
	Packages       []PackageBaseline `json:"packages"`
	Source         string            `json:"source"`
	FirmwarePrefix string            `json:"firmwarePrefix"`
}
type CellBaseline struct {
	Radio        string    `json:"radio"`
	MCC          string    `json:"mcc"`
	MNC          string    `json:"mnc"`
	Area         int64     `json:"area"`
	CellID       int64     `json:"cellId"`
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	RadiusMeters float64   `json:"radiusMeters"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Source       string    `json:"source"`
}
type RegionBoundary struct {
	Province string          `json:"province"`
	Polygons [][][][]float64 `json:"polygons"`
}
type ResourceCatalog struct {
	Version string           `json:"version"`
	Devices []DeviceBaseline `json:"devices"`
	Cells   []CellBaseline   `json:"cells"`
	Regions []RegionBoundary `json:"regions"`
}
type ResourceEngine struct {
	Catalog ResourceCatalog
	V4, V6  *IPDatabase
}

func LoadResources(catalog, v4, v6 string) (*ResourceEngine, error) {
	e := &ResourceEngine{Catalog: ResourceCatalog{Version: "resources-0.5"}}
	if catalog != "" {
		data, err := os.ReadFile(catalog)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &e.Catalog); err != nil {
			return nil, err
		}
		if e.Catalog.Version == "" {
			return nil, fmt.Errorf("resource catalog version required")
		}
	}
	var err error
	if v4 != "" {
		e.V4, err = LoadIPDatabase(v4)
		if err != nil {
			return nil, err
		}
		if e.V4.width != 4 {
			return nil, fmt.Errorf("IPv4 database expected")
		}
	}
	if v6 != "" {
		e.V6, err = LoadIPDatabase(v6)
		if err != nil {
			return nil, err
		}
		if e.V6.width != 16 {
			return nil, fmt.Errorf("IPv6 database expected")
		}
	}
	return e, nil
}
func signalObject(r Report, name string, target any) error {
	s, ok := r.Signals[name]
	if !ok || s.Status != "available" {
		return fmt.Errorf("%s: %s %s", name, s.Status, s.Reason)
	}
	data, err := json.Marshal(s.Value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
func equalText(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
func oneOf(v string, values []string) bool {
	for _, x := range values {
		if equalText(v, x) {
			return true
		}
	}
	return false
}

type hardwareObservation struct {
	Brand           string            `json:"brand"`
	Manufacturer    string            `json:"manufacturer"`
	Model           string            `json:"model"`
	Device          string            `json:"device"`
	Hardware        string            `json:"hardware"`
	Platform        string            `json:"platform"`
	Machine         string            `json:"machine"`
	MemoryBytes     float64           `json:"memoryBytes"`
	ConfiguredCores int               `json:"configuredCores"`
	AvailableCores  int               `json:"availableCores"`
	Errors          map[string]string `json:"errors"`
}
type packageObservation struct {
	Package string   `json:"package"`
	Status  string   `json:"status"`
	Signers []string `json:"signers"`
	System  bool     `json:"system"`
}
type gpsObservation struct {
	Latitude    float64  `json:"latitude"`
	Longitude   float64  `json:"longitude"`
	Accuracy    *float64 `json:"accuracyMeters"`
	Age         *float64 `json:"ageMillis"`
	Mock        bool     `json:"mock"`
	Coordinates string   `json:"coordinateSystem"`
}
type cellObservation struct {
	Radio      string   `json:"technology"`
	MCC        string   `json:"mcc"`
	MNC        string   `json:"mnc"`
	Area       int64    `json:"areaCode"`
	CellID     int64    `json:"cellId"`
	Registered bool     `json:"registered"`
	Age        *float64 `json:"ageMillis"`
}

func (e *ResourceEngine) Assess(r Report) (*ResourceAssessment, error) {
	result := &ResourceAssessment{Version: e.Catalog.Version, Checks: []ResourceCheck{}}
	add := func(code, state, dimension, detail, source string, points int) {
		result.Checks = append(result.Checks, ResourceCheck{code, state, dimension, detail, source, points})
	}
	var java, native hardwareObservation
	jErr, nErr := signalObject(r, "resourceJava", &java), signalObject(r, "resourceNative", &native)
	if jErr != nil || nErr != nil {
		add("hardware_crosscheck", "unknown", "hardware", fmt.Sprintf("java=%v; native=%v", jErr, nErr), "client", 0)
	} else {
		differences := []string{}
		comparable := 0
		for _, pair := range [][3]string{{"brand", java.Brand, native.Brand}, {"manufacturer", java.Manufacturer, native.Manufacturer}, {"model", java.Model, native.Model}, {"device", java.Device, native.Device}, {"hardware", java.Hardware, native.Hardware}} {
			if pair[1] != "" && pair[2] != "" {
				comparable++
				if !equalText(pair[1], pair[2]) {
					differences = append(differences, pair[0])
				}
			}
		}
		if len(differences) > 0 {
			add("build_native_mismatch", "conflict", "hardware", strings.Join(differences, ","), "Java/NDK", 20)
		} else if comparable > 0 {
			add("build_native_crosscheck", "matched", "hardware", "可比较的品牌、机型与硬件字段一致", "Java/NDK", 0)
		} else {
			add("hardware_crosscheck", "unknown", "hardware", "没有可比较的机型参数", "Java/NDK", 0)
		}
		if java.MemoryBytes > 0 && native.MemoryBytes > 0 {
			ratio := math.Abs(java.MemoryBytes-native.MemoryBytes) / native.MemoryBytes
			if ratio > 0.05 && math.Abs(java.MemoryBytes-native.MemoryBytes) > 128*1024*1024 {
				add("memory_source_mismatch", "conflict", "hardware", fmt.Sprintf("Java %.2f GiB / native %.2f GiB", java.MemoryBytes/(1<<30), native.MemoryBytes/(1<<30)), "Java/NDK", 20)
			} else {
				add("memory_source_crosscheck", "matched", "hardware", "内存读数在容差内", "Java/NDK", 0)
			}
		}
		if native.ConfiguredCores > 0 && java.AvailableCores > native.ConfiguredCores {
			add("cpu_core_mismatch", "conflict", "hardware", fmt.Sprintf("Java %d > native %d", java.AvailableCores, native.ConfiguredCores), "Java/NDK", 15)
		}
		if len(native.Errors) > 0 {
			b, _ := json.Marshal(native.Errors)
			add("native_resource_errors", "error", "hardware", string(b), "NDK", 0)
		}
	}
	var baseline *DeviceBaseline
	for i := range e.Catalog.Devices {
		candidate := &e.Catalog.Devices[i]
		if equalText(java.Brand, candidate.Brand) && equalText(java.Model, candidate.Model) {
			baseline = candidate
			break
		}
	}
	if baseline == nil {
		add("device_baseline", "unknown", "hardware", "当前机型尚无已配置基线", e.Catalog.Version, 0)
	} else {
		d := baseline
		add("device_baseline", "matched", "hardware", d.ID, d.Source, 0)
		mismatch := []string{}
		if d.Device != "" && java.Device != "" && !equalText(java.Device, d.Device) {
			mismatch = append(mismatch, "device")
		}
		if len(d.Hardware) > 0 && native.Hardware != "" && !oneOf(native.Hardware, d.Hardware) {
			mismatch = append(mismatch, "hardware")
		}
		if len(d.Platforms) > 0 && native.Platform != "" && !oneOf(native.Platform, d.Platforms) {
			mismatch = append(mismatch, "platform")
		}
		if d.Cores > 0 && native.ConfiguredCores > 0 && native.ConfiguredCores != d.Cores {
			mismatch = append(mismatch, "cpu cores")
		}
		if len(d.MemoryGiB) > 0 && native.MemoryBytes > 0 {
			valid := false
			for _, size := range d.MemoryGiB {
				ratio := native.MemoryBytes / (size * (1 << 30))
				if ratio >= 0.75 && ratio <= 1.03 {
					valid = true
				}
			}
			if !valid {
				mismatch = append(mismatch, "RAM SKU")
			}
		}
		if len(mismatch) > 0 {
			add("device_spec_mismatch", "conflict", "hardware", strings.Join(mismatch, ","), d.Source, 25)
		}
		fingerprint, _ := r.Signals["buildFingerprint"].Value.(string)
		if d.FirmwarePrefix != "" && !strings.HasPrefix(fingerprint, d.FirmwarePrefix) {
			add("firmware_baseline", "unknown", "software", "当前固件不在应用签名基线范围", d.Source, 0)
		} else {
			var packages []packageObservation
			if err := signalObject(r, "systemPackages", &packages); err != nil {
				add("package_baseline", "unknown", "software", err.Error(), d.Source, 0)
			} else {
				for _, expected := range d.Packages {
					var actual *packageObservation
					for i := range packages {
						if packages[i].Package == expected.Package {
							actual = &packages[i]
							break
						}
					}
					if actual == nil || actual.Status != "visible" {
						if expected.Required {
							add("system_package_coverage", "unknown", "software", expected.Package+": absent or filtered", d.Source, 0)
						}
						continue
					}
					valid := len(actual.Signers) > 0
					for _, signer := range actual.Signers {
						if !oneOf(signer, expected.Signers) {
							valid = false
						}
					}
					if !valid {
						add("system_package_signer_mismatch", "conflict", "software", expected.Package, d.Source, 25)
					} else {
						add("system_package_signer", "matched", "software", expected.Package, d.Source, 0)
					}
				}
			}
		}
	}
	ip, err := netip.ParseAddr(r.ObservedIP)
	if err != nil {
		return nil, err
	}
	if ip.IsPrivate() || ip.IsLoopback() || !ip.IsGlobalUnicast() {
		add("ip_region", "unknown", "network", "非公网地址", "server", 0)
	} else {
		db := e.V6
		if ip.Unmap().Is4() {
			db = e.V4
		}
		if db == nil {
			add("ip_region", "unknown", "network", "该地址族的归属库未配置", "server", 0)
		} else {
			result.IP, err = db.Lookup(ip)
			if err != nil {
				return nil, err
			}
			if result.IP == nil {
				add("ip_region", "unknown", "network", "归属库未命中", "ip2region", 0)
			} else {
				add("ip_region", "matched", "network", result.IP.Country+" / "+result.IP.Province+" / "+result.IP.City+" / "+result.IP.ISP, "ip2region", 0)
			}
		}
	}
	var gps gpsObservation
	gpsErr := signalObject(r, "location", &gps)
	fresh := gpsErr == nil && gps.Age != nil && *gps.Age >= 0 && *gps.Age <= 120000 && gps.Accuracy != nil && *gps.Accuracy > 0 && *gps.Accuracy <= 5000 && gps.Latitude >= -90 && gps.Latitude <= 90 && gps.Longitude >= -180 && gps.Longitude <= 180 && gps.Coordinates == "WGS84"
	if gpsErr != nil {
		add("gps_coverage", "unknown", "network", gpsErr.Error(), "client", 0)
	} else if gps.Mock {
		add("mock_location", "conflict", "network", "系统标记为模拟位置", "client", 20)
	} else if !fresh {
		add("gps_coverage", "stale", "network", "定位时效、精度或坐标系不满足比较条件", "client", 0)
	}
	var cells []cellObservation
	if err := signalObject(r, "cell", &cells); err != nil {
		add("cell_coverage", "unknown", "network", err.Error(), "client", 0)
	} else {
		matched := false
		for _, cell := range cells {
			if !cell.Registered || cell.Age == nil || *cell.Age < 0 || *cell.Age > 120000 {
				continue
			}
			for _, known := range e.Catalog.Cells {
				if cell.Radio != known.Radio || cell.MCC != known.MCC || cell.MNC != known.MNC || cell.Area != known.Area || cell.CellID != known.CellID {
					continue
				}
				matched = true
				if time.Since(known.UpdatedAt) > 180*24*time.Hour || known.UpdatedAt.After(time.Now()) || known.RadiusMeters <= 0 || known.Source == "" {
					add("cell_location", "stale", "network", "基站坐标来源、覆盖半径或时效不足", known.Source, 0)
					continue
				}
				if fresh && !gps.Mock {
					distance := distanceMeters(gps.Latitude, gps.Longitude, known.Latitude, known.Longitude)
					threshold := math.Max(known.RadiusMeters, 10000) + *gps.Accuracy
					if distance > threshold {
						add("cell_gps_mismatch", "conflict", "network", fmt.Sprintf("相距 %.0f km，比较半径 %.0f km", distance/1000, threshold/1000), known.Source, 25)
					} else {
						add("cell_gps_crosscheck", "matched", "network", "定位位于已知基站覆盖容差内", known.Source, 0)
					}
				}
			}
		}
		if !matched {
			add("cell_location", "unknown", "network", "没有对应的可信基站坐标，未进行距离判断", e.Catalog.Version, 0)
		}
	}
	if fresh && !gps.Mock && result.IP != nil {
		province, border := locateRegion(gps.Latitude, gps.Longitude, e.Catalog.Regions)
		vpn := r.Signals["vpn"].Value == true
		transport, _ := r.Signals["transport"].Value.(string)
		if province == "" {
			add("ip_gps_region", "unknown", "network", "定位区域不在当前地理资源覆盖内", e.Catalog.Version, 0)
		} else if result.IP.CountryCode == "CN" && normalizeProvince(province) == normalizeProvince(result.IP.Province) {
			add("ip_gps_region", "matched", "network", province, "IP归属/地理区域", 0)
		} else if vpn || transport == "cellular" || border < 100000 || result.IP.Province == "" {
			add("ip_gps_region", "unknown", "network", "VPN、移动网络出口、区域边界或归属粒度不适合强比较", "IP归属/地理区域", 0)
		} else {
			add("ip_gps_region_mismatch", "conflict", "network", "GPS="+province+"; IP="+result.IP.Province, "IP归属/地理区域", 10)
		}
	}
	return result, nil
}
func normalizeProvince(v string) string {
	for _, suffix := range []string{"维吾尔自治区", "壮族自治区", "回族自治区", "特别行政区", "自治区", "省", "市"} {
		v = strings.TrimSuffix(v, suffix)
	}
	return v
}
func distanceMeters(a, b, c, d float64) float64 {
	r := math.Pi / 180
	x := math.Sin((c - a) * r / 2)
	y := math.Sin((d - b) * r / 2)
	h := x*x + math.Cos(a*r)*math.Cos(c*r)*y*y
	return 6371000 * 2 * math.Asin(math.Sqrt(math.Min(1, h)))
}
func locateRegion(lat, lon float64, regions []RegionBoundary) (string, float64) {
	for _, region := range regions {
		for _, polygon := range region.Polygons {
			inside := false
			border := math.MaxFloat64
			for _, ring := range polygon {
				if len(ring) < 3 {
					continue
				}
				hit := false
				for i, j := 0, len(ring)-1; i < len(ring); j, i = i, i+1 {
					a, b := ring[i], ring[j]
					if len(a) != 2 || len(b) != 2 {
						continue
					}
					if (a[1] > lat) != (b[1] > lat) && lon < (b[0]-a[0])*(lat-a[1])/(b[1]-a[1])+a[0] {
						hit = !hit
					}
					scale := math.Cos(lat * math.Pi / 180)
					ax, ay := (a[0]-lon)*scale, a[1]-lat
					bx, by := (b[0]-lon)*scale, b[1]-lat
					dx, dy := bx-ax, by-ay
					t := 0.0
					if dx*dx+dy*dy > 0 {
						t = math.Max(0, math.Min(1, -(ax*dx+ay*dy)/(dx*dx+dy*dy)))
					}
					border = math.Min(border, math.Hypot(ax+t*dx, ay+t*dy)*111195)
				}
				if hit {
					inside = !inside
				}
			}
			if inside {
				return region.Province, border
			}
		}
	}
	return "", 0
}
