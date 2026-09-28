package pipes

import (
	"net"
	"reflect"
	"testing"

	gomail "gopkg.in/mail.v2"

	"nukumizu-backend/config"
)

// dialerIsDefault reports whether gomail still dials directly. Function values
// are not comparable in Go, so the code pointers are compared instead.
func dialerIsDefault() bool {
	return reflect.ValueOf(gomail.NetDialTimeout).Pointer() ==
		reflect.ValueOf(net.DialTimeout).Pointer()
}

// TestApplyEmailProxyRestoresDefaultDialer covers the case the constructor used
// to get wrong. gomail exposes its dial hook as a package-level variable with no
// unset, and the code only installed the proxy dialer when the flag was set, so
// turning networkUseProxy back off left SMTP tunnelled through a proxy nobody
// had asked for — with no way to undo it short of a restart.
func TestApplyEmailProxyRestoresDefaultDialer(t *testing.T) {
	t.Cleanup(func() { gomail.NetDialTimeout = net.DialTimeout })

	applyEmailProxy(true)
	if dialerIsDefault() {
		t.Fatal("applyEmailProxy(true) did not install the proxy dialer")
	}

	applyEmailProxy(false)
	if !dialerIsDefault() {
		t.Error("applyEmailProxy(false) left the proxy dialer in place")
	}
}

// TestNewEmailControllerAppliesProxySetting pins that the dialer follows the
// settings a controller is built with, which is what makes a rebuilt controller
// pick up a changed proxy flag.
func TestNewEmailControllerAppliesProxySetting(t *testing.T) {
	t.Cleanup(func() { gomail.NetDialTimeout = net.DialTimeout })

	NewEmailController(config.EmailConfig{NetworkUseProxy: true})
	if dialerIsDefault() {
		t.Error("a controller built with networkUseProxy: true did not install the proxy dialer")
	}

	NewEmailController(config.EmailConfig{NetworkUseProxy: false})
	if !dialerIsDefault() {
		t.Error("a controller rebuilt with networkUseProxy: false left the proxy dialer in place")
	}
}
