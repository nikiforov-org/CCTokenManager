package main

// The tokens on macOS: each is a password in the login keychain, of the service
// CC Token Manager and the account of its profile's id.

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef cp_item(const char *id) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef account = CFStringCreateWithCString(NULL, id, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, CFSTR("CC Token Manager"));
	CFDictionarySetValue(q, kSecAttrAccount, account);
	CFRelease(account);
	return q;
}

// cp_token_load hands back a copy of the token, for the caller to free.
static OSStatus cp_token_load(const char *id, char **token, long *len) {
	CFMutableDictionaryRef q = cp_item(id);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDataRef data = NULL;
	OSStatus s = SecItemCopyMatching(q, (CFTypeRef *)&data);
	CFRelease(q);
	if (s == errSecSuccess) {
		*len = CFDataGetLength(data);
		*token = malloc(*len + 1);
		memcpy(*token, CFDataGetBytePtr(data), *len);
		CFRelease(data);
	}
	return s;
}

// cp_token_store replaces the token, or puts it in if there is none yet. An
// empty one removes it.
static OSStatus cp_token_store(const char *id, const char *token, long len) {
	CFMutableDictionaryRef q = cp_item(id);
	OSStatus s;
	if (len == 0) {
		s = SecItemDelete(q);
		if (s == errSecItemNotFound)
			s = errSecSuccess;
	} else {
		CFDataRef data = CFDataCreate(NULL, (const UInt8 *)token, len);
		CFDictionaryRef change = CFDictionaryCreate(NULL,
			(const void **)&kSecValueData, (const void **)&data, 1,
			&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		s = SecItemUpdate(q, change);
		if (s == errSecItemNotFound) {
			CFDictionarySetValue(q, kSecValueData, data);
			s = SecItemAdd(q, NULL);
		}
		CFRelease(change);
		CFRelease(data);
	}
	CFRelease(q);
	return s;
}

static void cp_status(OSStatus s, char *buf, int size) {
	CFStringRef m = SecCopyErrorMessageString(s, NULL);
	if (!m || !CFStringGetCString(m, buf, size, kCFStringEncodingUTF8))
		snprintf(buf, size, "error %d", (int)s);
	if (m)
		CFRelease(m);
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// storeName is what the system calls the place the tokens are kept.
const storeName = "The Keychain"

// loadToken is the profile's token, or "" if the keychain has none for it.
func loadToken(id string) (string, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	var token *C.char
	var n C.long
	switch s := C.cp_token_load(cid, &token, &n); s {
	case C.errSecSuccess:
		defer C.free(unsafe.Pointer(token))
		return C.GoStringN(token, C.int(n)), nil
	case C.errSecItemNotFound:
		return "", nil
	default:
		return "", keychainError(s)
	}
}

// storeToken puts the profile's token in the keychain, and an empty one takes
// it out.
func storeToken(id, token string) error {
	cid, ctok := C.CString(id), C.CString(token)
	defer C.free(unsafe.Pointer(cid))
	defer C.free(unsafe.Pointer(ctok))
	if s := C.cp_token_store(cid, ctok, C.long(len(token))); s != C.errSecSuccess {
		return keychainError(s)
	}
	return nil
}

func keychainError(s C.OSStatus) error {
	var buf [256]C.char
	C.cp_status(s, &buf[0], C.int(len(buf)))
	return errors.New(C.GoString(&buf[0]))
}
