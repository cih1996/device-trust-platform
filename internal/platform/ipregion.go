package platform

// XDB vector/segment layout adapted from Ip2Region (Apache-2.0), pinned at
// 92e6a788d17c3185c852ca983124c3fca280f992. Immutable buffers, no shared seek cursor.
import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"strings"
	"time"
)

type IPRegion struct {
	Country     string    `json:"country"`
	Province    string    `json:"province"`
	City        string    `json:"city"`
	ISP         string    `json:"isp"`
	CountryCode string    `json:"countryCode"`
	Source      string    `json:"source"`
	BuiltAt     time.Time `json:"builtAt"`
}
type IPDatabase struct {
	data    []byte
	width   int
	BuiltAt time.Time
}

func LoadIPDatabase(path string) (*IPDatabase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 524544 {
		return nil, errors.New("XDB header/vector truncated")
	}
	version := binary.LittleEndian.Uint16(data)
	width := 4
	if version == 3 {
		if binary.LittleEndian.Uint16(data[16:]) == 6 {
			width = 16
		} else if binary.LittleEndian.Uint16(data[16:]) != 4 {
			return nil, errors.New("invalid XDB IP version")
		}
		if binary.LittleEndian.Uint16(data[18:]) != 4 {
			return nil, errors.New("XDB pointer size unsupported")
		}
	} else if version != 2 {
		return nil, errors.New("unsupported XDB structure")
	}
	if binary.LittleEndian.Uint16(data[2:]) != 1 {
		return nil, errors.New("XDB vector index required")
	}
	return &IPDatabase{data, width, time.Unix(int64(binary.LittleEndian.Uint32(data[4:])), 0).UTC()}, nil
}
func (d *IPDatabase) Lookup(ip netip.Addr) (*IPRegion, error) {
	ip = ip.Unmap()
	key := ip.AsSlice()
	if len(key) != d.width {
		return nil, nil
	}
	index := 256 + (int(key[0])*256+int(key[1]))*8
	start, end := int(binary.LittleEndian.Uint32(d.data[index:])), int(binary.LittleEndian.Uint32(d.data[index+4:]))
	if start == 0 && end == 0 {
		return nil, nil
	}
	size := 2*d.width + 6
	if start < 524544 || end < start || end+size > len(d.data) || (end-start)%size != 0 {
		return nil, errors.New("corrupt XDB segment range")
	}
	compare := func(a, b []byte) int {
		if d.width == 16 {
			return bytes.Compare(a, b)
		}
		for i := 0; i < 4; i++ {
			if a[i] < b[3-i] {
				return -1
			}
			if a[i] > b[3-i] {
				return 1
			}
		}
		return 0
	}
	for lo, hi := 0, (end-start)/size; lo <= hi; {
		mid := (lo + hi) / 2
		row := d.data[start+mid*size : start+(mid+1)*size]
		if compare(key, row[:d.width]) < 0 {
			hi = mid - 1
		} else if compare(key, row[d.width:2*d.width]) > 0 {
			lo = mid + 1
		} else {
			n := int(binary.LittleEndian.Uint16(row[2*d.width:]))
			p := int(binary.LittleEndian.Uint32(row[2*d.width+2:]))
			if p < 256 || p+n > len(d.data) {
				return nil, errors.New("corrupt XDB data pointer")
			}
			fields := strings.Split(string(d.data[p:p+n]), "|")
			if len(fields) != 5 {
				return nil, errors.New("XDB region schema must be Country|Province|City|ISP|ISO2")
			}
			for i := range fields {
				if fields[i] == "0" {
					fields[i] = ""
				}
			}
			return &IPRegion{fields[0], fields[1], fields[2], fields[3], fields[4], "ip2region", d.BuiltAt}, nil
		}
	}
	return nil, nil
}
