package constants

import "testing"

func TestConstantsIncludesDocumentedFSValues(t *testing.T) {
	for name, want := range map[string]float64{
		"F_OK":                   0,
		"R_OK":                   4,
		"W_OK":                   2,
		"X_OK":                   1,
		"O_RDONLY":               0,
		"O_WRONLY":               1,
		"O_RDWR":                 2,
		"COPYFILE_EXCL":          1,
		"COPYFILE_FICLONE":       2,
		"UV_DIRENT_FILE":         1,
		"UV_DIRENT_DIR":          2,
		"UV_FS_SYMLINK_DIR":      1,
		"UV_FS_SYMLINK_JUNCTION": 2,
	} {
		if got := AsJSValue.Get(name).Number(); got != want {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
}

func TestConstantsIncludesDocumentedCryptoValues(t *testing.T) {
	for name, want := range map[string]float64{
		"RSA_PKCS1_PADDING":       1,
		"RSA_NO_PADDING":          3,
		"RSA_PKCS1_OAEP_PADDING":  4,
		"RSA_X931_PADDING":        5,
		"RSA_PKCS1_PSS_PADDING":   6,
		"RSA_PSS_SALTLEN_DIGEST":  -1,
		"POINT_CONVERSION_HYBRID": 6,
		"ENGINE_METHOD_RSA":       1,
		"ENGINE_METHOD_ALL":       65535,
		"TLS1_3_VERSION":          772,
	} {
		if got := AsJSValue.Get(name).Number(); got != want {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
	if AsJSValue.Get("defaultCoreCipherList").TypeString() != "string" {
		t.Fatal("expected defaultCoreCipherList string")
	}
}

func TestOSConstantsGroupsMatchBunReferenceShape(t *testing.T) {
	dlopen := OSConstants.Get("dlopen")
	if dlopen.Get("RTLD_LAZY").Number() != 1 || dlopen.Get("RTLD_NOW").Number() != 2 || dlopen.Get("RTLD_LOCAL").Number() != 4 || dlopen.Get("RTLD_GLOBAL").Number() != 8 {
		t.Fatal("unexpected dlopen constants")
	}
	errno := OSConstants.Get("errno")
	if errno.Get("ENOENT").Number() != 2 || errno.Get("EACCES").Number() != 13 || errno.Get("EISDIR").Number() != 21 {
		t.Fatal("unexpected errno constants")
	}
	priority := OSConstants.Get("priority")
	if priority.Get("PRIORITY_LOW").Number() != 19 || priority.Get("PRIORITY_NORMAL").Number() != 0 || priority.Get("PRIORITY_HIGHEST").Number() != -20 {
		t.Fatal("unexpected priority constants")
	}
	signals := OSConstants.Get("signals")
	if signals.Get("SIGINT").Number() != 2 || signals.Get("SIGTERM").Number() != 15 || signals.Get("SIGKILL").Number() != 9 {
		t.Fatal("unexpected signal constants")
	}
}
