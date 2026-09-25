package main

// The tokens on Linux: each is a secret in the keyring, through the Secret
// Service (GNOME Keyring, KWallet), under the app's schema and its profile's id.

/*
#cgo pkg-config: libsecret-1
#include <libsecret/secret.h>
#include <stdlib.h>
#include <string.h>

static const SecretSchema *cp_schema(void) {
	static const SecretSchema schema = {
		"org.nikiforov.CCTokenManager", SECRET_SCHEMA_NONE,
		{{"id", SECRET_SCHEMA_ATTRIBUTE_STRING}, {NULL, 0}},
	};
	return &schema;
}

// cp_error hands back the error's message for the caller to free, and frees it.
static char *cp_error(GError *e) {
	char *m = strdup(e->message);
	g_error_free(e);
	return m;
}

static char *cp_secret_load(const char *id, char **err) {
	GError *e = NULL;
	gchar *p = secret_password_lookup_sync(cp_schema(), NULL, &e, "id", id, NULL);
	if (e != NULL) {
		*err = cp_error(e);
		return NULL;
	}
	char *out = p ? strdup(p) : NULL;
	secret_password_free(p);
	return out;
}

static char *cp_secret_store(const char *id, const char *value) {
	GError *e = NULL;
	if (*value == 0)
		secret_password_clear_sync(cp_schema(), NULL, &e, "id", id, NULL);
	else {
		gchar *label = g_strdup_printf("CC Token Manager %s", id);
		secret_password_store_sync(cp_schema(), SECRET_COLLECTION_DEFAULT, label, value,
			NULL, &e, "id", id, NULL);
		g_free(label);
	}
	return e != NULL ? cp_error(e) : NULL;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// storeName is what the system calls the place the tokens are kept.
const storeName = "The keyring"

// loadToken is the profile's token, or "" if the keyring has none for it.
func loadToken(id string) (string, error) {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	var cerr *C.char
	p := C.cp_secret_load(cid, &cerr)
	if cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return "", errors.New(C.GoString(cerr))
	}
	if p == nil {
		return "", nil
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), nil
}

// storeToken puts the profile's token in the keyring, and an empty one takes it
// out.
func storeToken(id, token string) error {
	cid, ctok := C.CString(id), C.CString(token)
	defer C.free(unsafe.Pointer(cid))
	defer C.free(unsafe.Pointer(ctok))
	if cerr := C.cp_secret_store(cid, ctok); cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return errors.New(C.GoString(cerr))
	}
	return nil
}
