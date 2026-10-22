package licensecheck

import "testing"

// TestDetectLicense 校验许可证文本识别覆盖常见许可证。
func TestDetectLicense(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "apache", text: "Apache License\nVersion 2.0, January 2004", want: "Apache-2.0"},
		{name: "mit", text: "MIT License\nPermission is hereby granted, free of charge", want: "MIT"},
		{name: "isc", text: "Permission to use, copy, modify, and distribute this software", want: "ISC"},
		{name: "bsd3", text: "Redistribution and use in source and binary forms, with or without modification, are permitted provided that neither the name", want: "BSD-3-Clause"},
		{name: "bsd2", text: "Redistribution and use in source and binary forms, with or without modification", want: "BSD-2-Clause"},
		{name: "mpl", text: "Mozilla Public License Version 2.0", want: "MPL-2.0"},
		{name: "epl", text: "Eclipse Public License - v 2.0", want: "EPL-2.0"},
		{name: "agpl", text: "GNU AFFERO GENERAL PUBLIC LICENSE", want: "AGPL-3.0"},
		{name: "lgpl", text: "GNU LESSER GENERAL PUBLIC LICENSE", want: "LGPL"},
		{name: "gpl", text: "GNU GENERAL PUBLIC LICENSE", want: "GPL"},
		{name: "unknown", text: "All rights reserved.", want: "UNKNOWN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectLicense(tc.text); got != tc.want {
				t.Errorf("detectLicense() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLicenseAllowed 校验许可证表达式求值：OR 任一允许即可，AND 要求全部允许。
func TestLicenseAllowed(t *testing.T) {
	cases := []struct {
		expr string
		want bool
	}{
		{"MIT", true},
		{"Apache-2.0", true},
		{"(MPL-2.0 OR Apache-2.0)", true},
		{"(MIT OR CC0-1.0)", true},
		{"(Apache-2.0 AND BSD-3-Clause)", true},
		{"(GPL-3.0 OR MIT)", true},
		{"GPL-3.0", false},
		{"LGPL-2.1", false},
		{"AGPL-3.0", false},
		{"SSPL-1.0", false},
		{"UNKNOWN", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := licenseAllowed(tc.expr); got != tc.want {
			t.Errorf("licenseAllowed(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestRejectReason 校验拒绝原因：强 copyleft 与未知许可证必须被拦截，宽松许可证放行。
func TestRejectReason(t *testing.T) {
	cases := []struct {
		license string
		reject  bool
	}{
		{"MIT", false},
		{"EPL-2.0", false},
		{"MPL-2.0", false},
		{"GPL-3.0", true},
		{"LGPL-2.1", true},
		{"AGPL-3.0", true},
		{"BUSL-1.1", true},
		{"UNKNOWN", true},
		{"", true},
	}
	for _, tc := range cases {
		reason := rejectReason(tc.license)
		if (reason != "") != tc.reject {
			t.Errorf("rejectReason(%q) = %q, want reject=%v", tc.license, reason, tc.reject)
		}
	}
}

// TestNormalizeNpmLicense 校验 npm 许可证写法归一化与已知缺失元数据的补全。
func TestNormalizeNpmLicense(t *testing.T) {
	cases := []struct {
		name     string
		version  string
		declared string
		want     string
	}{
		{"pkg", "1.0.0", "MIT/X11", "MIT"},
		{"pkg", "1.0.0", "BSD", "BSD-3-Clause"},
		{"khroma", "2.1.0", "Unknown", "MIT"},
		{"pkg", "1.0.0", "Unknown", "UNKNOWN"},
		{"pkg", "1.0.0", "(MPL-2.0 OR Apache-2.0)", "(MPL-2.0 OR Apache-2.0)"},
	}
	for _, tc := range cases {
		if got := normalizeNpmLicense(tc.name, tc.version, tc.declared); got != tc.want {
			t.Errorf("normalizeNpmLicense(%q,%q,%q) = %q, want %q", tc.name, tc.version, tc.declared, got, tc.want)
		}
	}
}
