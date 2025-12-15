package main

import "testing"

func Test_isNewer_Release(t *testing.T) {
	isSameSign := func(a, b int) bool {
		return (a == 0 && b == 0) || (a > 0 && b > 0) || (a < 0 && b < 0)
	}
	verifyOk := func(ver1, ver2 string, expect int) {
		parsed1 := ParseVersion(ver1)
		parsed2 := ParseVersion(ver2)
		if cmp, err := parsed1.Compare(parsed2); err != nil ||
			!isSameSign(cmp, expect) {
			t.Error(ver1 + " should not be newer than " + ver2)
		}
		if cmp, err := parsed2.Compare(parsed1); err != nil ||
			!isSameSign(cmp, -expect) {
			t.Error(ver2 + " should be newer than " + ver1)
		}
	}
	verifyOk("v0.0.1", "v0.0.2", -1)
	verifyOk("v0.0.1", "v0.0.1", 0)
	verifyOk("v0.0.1", "v0.1.1", -1)
	verifyOk("v0.0.1", "v0.0.1-rc1", 1)
	verifyOk("v0.0.1", "v0.0.2-rc1", -1)
	verifyOk("v0.0.1", "v0.0.0-rc1", 1)
	verifyOk("v0.0.1-rc4", "v0.0.0-beta19", 1)
	verifyOk("v0.0.1-alpha24", "v0.0.0-beta19", 1)
	verifyOk("v0.0.1-rc0", "v0.0.1-rc1", -1)
}

func Test_ParseVersion(t *testing.T) {
	verifyOk := func(v string, ver [3]int, pre *Prerelease) {
		vs := ParseVersion(v)
		if !vs.Parsed {
			t.Error("Parse should succeed")
		}
		if vs.Numbers[0] != ver[0] ||
			vs.Numbers[1] != ver[1] ||
			vs.Numbers[2] != ver[2] {
			t.Error("Version mismatch")
		}
		if vs.Pre == nil && pre == nil {
			// Match
		} else if vs.Pre == nil || pre == nil {
			t.Error("PreRelease mismatch")
		} else if vs.Pre.Compare(*pre) != 0 {
			t.Error("PreRelease mismatch")
		}
	}
	verifyOk("v42.8.167", [3]int{42, 8, 167}, nil)
	verifyOk("v9999", [3]int{9999, 0, 0}, nil)
	verifyOk("v1.2.3-rc123", [3]int{1, 2, 3}, &Prerelease{t: PrereleaseRC, version: 123})
	verifyOk("v1.2.3-alpha1", [3]int{1, 2, 3}, &Prerelease{t: PrereleaseAlpha, version: 1})
	verifyOk("v1.2.3-beta0", [3]int{1, 2, 3}, &Prerelease{t: PrereleaseBeta, version: 0})
	verifyOk("v12345.1-rc123", [3]int{12345, 1, 0}, &Prerelease{t: PrereleaseRC, version: 123})

	verifyFail := func(v string) {
		vs := ParseVersion(v)
		if vs.Parsed {
			t.Error("Parse should fail")
		}
		if vs.Original != v {
			t.Error("Original should match")
		}
	}
	verifyFail("v0.0.1-rel0")
	verifyFail("v0.0.0.1")
}
