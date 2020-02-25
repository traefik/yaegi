// Code generated by 'goexports net/http'. DO NOT EDIT.

// +build go1.14,!go1.15

package stdlib

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
)

func init() {
	Symbols["net/http"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"CanonicalHeaderKey":                  reflect.ValueOf(http.CanonicalHeaderKey),
		"DefaultClient":                       reflect.ValueOf(&http.DefaultClient).Elem(),
		"DefaultMaxHeaderBytes":               reflect.ValueOf(http.DefaultMaxHeaderBytes),
		"DefaultMaxIdleConnsPerHost":          reflect.ValueOf(http.DefaultMaxIdleConnsPerHost),
		"DefaultServeMux":                     reflect.ValueOf(&http.DefaultServeMux).Elem(),
		"DefaultTransport":                    reflect.ValueOf(&http.DefaultTransport).Elem(),
		"DetectContentType":                   reflect.ValueOf(http.DetectContentType),
		"ErrAbortHandler":                     reflect.ValueOf(&http.ErrAbortHandler).Elem(),
		"ErrBodyNotAllowed":                   reflect.ValueOf(&http.ErrBodyNotAllowed).Elem(),
		"ErrBodyReadAfterClose":               reflect.ValueOf(&http.ErrBodyReadAfterClose).Elem(),
		"ErrContentLength":                    reflect.ValueOf(&http.ErrContentLength).Elem(),
		"ErrHandlerTimeout":                   reflect.ValueOf(&http.ErrHandlerTimeout).Elem(),
		"ErrHeaderTooLong":                    reflect.ValueOf(&http.ErrHeaderTooLong).Elem(),
		"ErrHijacked":                         reflect.ValueOf(&http.ErrHijacked).Elem(),
		"ErrLineTooLong":                      reflect.ValueOf(&http.ErrLineTooLong).Elem(),
		"ErrMissingBoundary":                  reflect.ValueOf(&http.ErrMissingBoundary).Elem(),
		"ErrMissingContentLength":             reflect.ValueOf(&http.ErrMissingContentLength).Elem(),
		"ErrMissingFile":                      reflect.ValueOf(&http.ErrMissingFile).Elem(),
		"ErrNoCookie":                         reflect.ValueOf(&http.ErrNoCookie).Elem(),
		"ErrNoLocation":                       reflect.ValueOf(&http.ErrNoLocation).Elem(),
		"ErrNotMultipart":                     reflect.ValueOf(&http.ErrNotMultipart).Elem(),
		"ErrNotSupported":                     reflect.ValueOf(&http.ErrNotSupported).Elem(),
		"ErrServerClosed":                     reflect.ValueOf(&http.ErrServerClosed).Elem(),
		"ErrShortBody":                        reflect.ValueOf(&http.ErrShortBody).Elem(),
		"ErrSkipAltProtocol":                  reflect.ValueOf(&http.ErrSkipAltProtocol).Elem(),
		"ErrUnexpectedTrailer":                reflect.ValueOf(&http.ErrUnexpectedTrailer).Elem(),
		"ErrUseLastResponse":                  reflect.ValueOf(&http.ErrUseLastResponse).Elem(),
		"ErrWriteAfterFlush":                  reflect.ValueOf(&http.ErrWriteAfterFlush).Elem(),
		"Error":                               reflect.ValueOf(http.Error),
		"FileServer":                          reflect.ValueOf(http.FileServer),
		"Get":                                 reflect.ValueOf(http.Get),
		"Handle":                              reflect.ValueOf(http.Handle),
		"HandleFunc":                          reflect.ValueOf(http.HandleFunc),
		"Head":                                reflect.ValueOf(http.Head),
		"ListenAndServe":                      reflect.ValueOf(http.ListenAndServe),
		"ListenAndServeTLS":                   reflect.ValueOf(http.ListenAndServeTLS),
		"LocalAddrContextKey":                 reflect.ValueOf(&http.LocalAddrContextKey).Elem(),
		"MaxBytesReader":                      reflect.ValueOf(http.MaxBytesReader),
		"MethodConnect":                       reflect.ValueOf(http.MethodConnect),
		"MethodDelete":                        reflect.ValueOf(http.MethodDelete),
		"MethodGet":                           reflect.ValueOf(http.MethodGet),
		"MethodHead":                          reflect.ValueOf(http.MethodHead),
		"MethodOptions":                       reflect.ValueOf(http.MethodOptions),
		"MethodPatch":                         reflect.ValueOf(http.MethodPatch),
		"MethodPost":                          reflect.ValueOf(http.MethodPost),
		"MethodPut":                           reflect.ValueOf(http.MethodPut),
		"MethodTrace":                         reflect.ValueOf(http.MethodTrace),
		"NewFileTransport":                    reflect.ValueOf(http.NewFileTransport),
		"NewRequest":                          reflect.ValueOf(http.NewRequest),
		"NewRequestWithContext":               reflect.ValueOf(http.NewRequestWithContext),
		"NewServeMux":                         reflect.ValueOf(http.NewServeMux),
		"NoBody":                              reflect.ValueOf(&http.NoBody).Elem(),
		"NotFound":                            reflect.ValueOf(http.NotFound),
		"NotFoundHandler":                     reflect.ValueOf(http.NotFoundHandler),
		"ParseHTTPVersion":                    reflect.ValueOf(http.ParseHTTPVersion),
		"ParseTime":                           reflect.ValueOf(http.ParseTime),
		"Post":                                reflect.ValueOf(http.Post),
		"PostForm":                            reflect.ValueOf(http.PostForm),
		"ProxyFromEnvironment":                reflect.ValueOf(http.ProxyFromEnvironment),
		"ProxyURL":                            reflect.ValueOf(http.ProxyURL),
		"ReadRequest":                         reflect.ValueOf(http.ReadRequest),
		"ReadResponse":                        reflect.ValueOf(http.ReadResponse),
		"Redirect":                            reflect.ValueOf(http.Redirect),
		"RedirectHandler":                     reflect.ValueOf(http.RedirectHandler),
		"SameSiteDefaultMode":                 reflect.ValueOf(http.SameSiteDefaultMode),
		"SameSiteLaxMode":                     reflect.ValueOf(http.SameSiteLaxMode),
		"SameSiteNoneMode":                    reflect.ValueOf(http.SameSiteNoneMode),
		"SameSiteStrictMode":                  reflect.ValueOf(http.SameSiteStrictMode),
		"Serve":                               reflect.ValueOf(http.Serve),
		"ServeContent":                        reflect.ValueOf(http.ServeContent),
		"ServeFile":                           reflect.ValueOf(http.ServeFile),
		"ServeTLS":                            reflect.ValueOf(http.ServeTLS),
		"ServerContextKey":                    reflect.ValueOf(&http.ServerContextKey).Elem(),
		"SetCookie":                           reflect.ValueOf(http.SetCookie),
		"StateActive":                         reflect.ValueOf(http.StateActive),
		"StateClosed":                         reflect.ValueOf(http.StateClosed),
		"StateHijacked":                       reflect.ValueOf(http.StateHijacked),
		"StateIdle":                           reflect.ValueOf(http.StateIdle),
		"StateNew":                            reflect.ValueOf(http.StateNew),
		"StatusAccepted":                      reflect.ValueOf(http.StatusAccepted),
		"StatusAlreadyReported":               reflect.ValueOf(http.StatusAlreadyReported),
		"StatusBadGateway":                    reflect.ValueOf(http.StatusBadGateway),
		"StatusBadRequest":                    reflect.ValueOf(http.StatusBadRequest),
		"StatusConflict":                      reflect.ValueOf(http.StatusConflict),
		"StatusContinue":                      reflect.ValueOf(http.StatusContinue),
		"StatusCreated":                       reflect.ValueOf(http.StatusCreated),
		"StatusEarlyHints":                    reflect.ValueOf(http.StatusEarlyHints),
		"StatusExpectationFailed":             reflect.ValueOf(http.StatusExpectationFailed),
		"StatusFailedDependency":              reflect.ValueOf(http.StatusFailedDependency),
		"StatusForbidden":                     reflect.ValueOf(http.StatusForbidden),
		"StatusFound":                         reflect.ValueOf(http.StatusFound),
		"StatusGatewayTimeout":                reflect.ValueOf(http.StatusGatewayTimeout),
		"StatusGone":                          reflect.ValueOf(http.StatusGone),
		"StatusHTTPVersionNotSupported":       reflect.ValueOf(http.StatusHTTPVersionNotSupported),
		"StatusIMUsed":                        reflect.ValueOf(http.StatusIMUsed),
		"StatusInsufficientStorage":           reflect.ValueOf(http.StatusInsufficientStorage),
		"StatusInternalServerError":           reflect.ValueOf(http.StatusInternalServerError),
		"StatusLengthRequired":                reflect.ValueOf(http.StatusLengthRequired),
		"StatusLocked":                        reflect.ValueOf(http.StatusLocked),
		"StatusLoopDetected":                  reflect.ValueOf(http.StatusLoopDetected),
		"StatusMethodNotAllowed":              reflect.ValueOf(http.StatusMethodNotAllowed),
		"StatusMisdirectedRequest":            reflect.ValueOf(http.StatusMisdirectedRequest),
		"StatusMovedPermanently":              reflect.ValueOf(http.StatusMovedPermanently),
		"StatusMultiStatus":                   reflect.ValueOf(http.StatusMultiStatus),
		"StatusMultipleChoices":               reflect.ValueOf(http.StatusMultipleChoices),
		"StatusNetworkAuthenticationRequired": reflect.ValueOf(http.StatusNetworkAuthenticationRequired),
		"StatusNoContent":                     reflect.ValueOf(http.StatusNoContent),
		"StatusNonAuthoritativeInfo":          reflect.ValueOf(http.StatusNonAuthoritativeInfo),
		"StatusNotAcceptable":                 reflect.ValueOf(http.StatusNotAcceptable),
		"StatusNotExtended":                   reflect.ValueOf(http.StatusNotExtended),
		"StatusNotFound":                      reflect.ValueOf(http.StatusNotFound),
		"StatusNotImplemented":                reflect.ValueOf(http.StatusNotImplemented),
		"StatusNotModified":                   reflect.ValueOf(http.StatusNotModified),
		"StatusOK":                            reflect.ValueOf(http.StatusOK),
		"StatusPartialContent":                reflect.ValueOf(http.StatusPartialContent),
		"StatusPaymentRequired":               reflect.ValueOf(http.StatusPaymentRequired),
		"StatusPermanentRedirect":             reflect.ValueOf(http.StatusPermanentRedirect),
		"StatusPreconditionFailed":            reflect.ValueOf(http.StatusPreconditionFailed),
		"StatusPreconditionRequired":          reflect.ValueOf(http.StatusPreconditionRequired),
		"StatusProcessing":                    reflect.ValueOf(http.StatusProcessing),
		"StatusProxyAuthRequired":             reflect.ValueOf(http.StatusProxyAuthRequired),
		"StatusRequestEntityTooLarge":         reflect.ValueOf(http.StatusRequestEntityTooLarge),
		"StatusRequestHeaderFieldsTooLarge":   reflect.ValueOf(http.StatusRequestHeaderFieldsTooLarge),
		"StatusRequestTimeout":                reflect.ValueOf(http.StatusRequestTimeout),
		"StatusRequestURITooLong":             reflect.ValueOf(http.StatusRequestURITooLong),
		"StatusRequestedRangeNotSatisfiable":  reflect.ValueOf(http.StatusRequestedRangeNotSatisfiable),
		"StatusResetContent":                  reflect.ValueOf(http.StatusResetContent),
		"StatusSeeOther":                      reflect.ValueOf(http.StatusSeeOther),
		"StatusServiceUnavailable":            reflect.ValueOf(http.StatusServiceUnavailable),
		"StatusSwitchingProtocols":            reflect.ValueOf(http.StatusSwitchingProtocols),
		"StatusTeapot":                        reflect.ValueOf(http.StatusTeapot),
		"StatusTemporaryRedirect":             reflect.ValueOf(http.StatusTemporaryRedirect),
		"StatusText":                          reflect.ValueOf(http.StatusText),
		"StatusTooEarly":                      reflect.ValueOf(http.StatusTooEarly),
		"StatusTooManyRequests":               reflect.ValueOf(http.StatusTooManyRequests),
		"StatusUnauthorized":                  reflect.ValueOf(http.StatusUnauthorized),
		"StatusUnavailableForLegalReasons":    reflect.ValueOf(http.StatusUnavailableForLegalReasons),
		"StatusUnprocessableEntity":           reflect.ValueOf(http.StatusUnprocessableEntity),
		"StatusUnsupportedMediaType":          reflect.ValueOf(http.StatusUnsupportedMediaType),
		"StatusUpgradeRequired":               reflect.ValueOf(http.StatusUpgradeRequired),
		"StatusUseProxy":                      reflect.ValueOf(http.StatusUseProxy),
		"StatusVariantAlsoNegotiates":         reflect.ValueOf(http.StatusVariantAlsoNegotiates),
		"StripPrefix":                         reflect.ValueOf(http.StripPrefix),
		"TimeFormat":                          reflect.ValueOf(http.TimeFormat),
		"TimeoutHandler":                      reflect.ValueOf(http.TimeoutHandler),
		"TrailerPrefix":                       reflect.ValueOf(http.TrailerPrefix),

		// type definitions
		"Client":         reflect.ValueOf((*http.Client)(nil)),
		"CloseNotifier":  reflect.ValueOf((*http.CloseNotifier)(nil)),
		"ConnState":      reflect.ValueOf((*http.ConnState)(nil)),
		"Cookie":         reflect.ValueOf((*http.Cookie)(nil)),
		"CookieJar":      reflect.ValueOf((*http.CookieJar)(nil)),
		"Dir":            reflect.ValueOf((*http.Dir)(nil)),
		"File":           reflect.ValueOf((*http.File)(nil)),
		"FileSystem":     reflect.ValueOf((*http.FileSystem)(nil)),
		"Flusher":        reflect.ValueOf((*http.Flusher)(nil)),
		"Handler":        reflect.ValueOf((*http.Handler)(nil)),
		"HandlerFunc":    reflect.ValueOf((*http.HandlerFunc)(nil)),
		"Header":         reflect.ValueOf((*http.Header)(nil)),
		"Hijacker":       reflect.ValueOf((*http.Hijacker)(nil)),
		"ProtocolError":  reflect.ValueOf((*http.ProtocolError)(nil)),
		"PushOptions":    reflect.ValueOf((*http.PushOptions)(nil)),
		"Pusher":         reflect.ValueOf((*http.Pusher)(nil)),
		"Request":        reflect.ValueOf((*http.Request)(nil)),
		"Response":       reflect.ValueOf((*http.Response)(nil)),
		"ResponseWriter": reflect.ValueOf((*http.ResponseWriter)(nil)),
		"RoundTripper":   reflect.ValueOf((*http.RoundTripper)(nil)),
		"SameSite":       reflect.ValueOf((*http.SameSite)(nil)),
		"ServeMux":       reflect.ValueOf((*http.ServeMux)(nil)),
		"Server":         reflect.ValueOf((*http.Server)(nil)),
		"Transport":      reflect.ValueOf((*http.Transport)(nil)),

		// interface wrapper definitions
		"_CloseNotifier":  reflect.ValueOf((*_net_http_CloseNotifier)(nil)),
		"_CookieJar":      reflect.ValueOf((*_net_http_CookieJar)(nil)),
		"_File":           reflect.ValueOf((*_net_http_File)(nil)),
		"_FileSystem":     reflect.ValueOf((*_net_http_FileSystem)(nil)),
		"_Flusher":        reflect.ValueOf((*_net_http_Flusher)(nil)),
		"_Handler":        reflect.ValueOf((*_net_http_Handler)(nil)),
		"_Hijacker":       reflect.ValueOf((*_net_http_Hijacker)(nil)),
		"_Pusher":         reflect.ValueOf((*_net_http_Pusher)(nil)),
		"_ResponseWriter": reflect.ValueOf((*_net_http_ResponseWriter)(nil)),
		"_RoundTripper":   reflect.ValueOf((*_net_http_RoundTripper)(nil)),
	}
}

// _net_http_CloseNotifier is an interface wrapper for CloseNotifier type
type _net_http_CloseNotifier struct {
	WCloseNotify func() <-chan bool
}

func (W _net_http_CloseNotifier) CloseNotify() <-chan bool { return W.WCloseNotify() }

// _net_http_CookieJar is an interface wrapper for CookieJar type
type _net_http_CookieJar struct {
	WCookies    func(u *url.URL) []*http.Cookie
	WSetCookies func(u *url.URL, cookies []*http.Cookie)
}

func (W _net_http_CookieJar) Cookies(u *url.URL) []*http.Cookie { return W.WCookies(u) }
func (W _net_http_CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	W.WSetCookies(u, cookies)
}

// _net_http_File is an interface wrapper for File type
type _net_http_File struct {
	WClose   func() error
	WRead    func(p []byte) (n int, err error)
	WReaddir func(count int) ([]os.FileInfo, error)
	WSeek    func(offset int64, whence int) (int64, error)
	WStat    func() (os.FileInfo, error)
}

func (W _net_http_File) Close() error                                 { return W.WClose() }
func (W _net_http_File) Read(p []byte) (n int, err error)             { return W.WRead(p) }
func (W _net_http_File) Readdir(count int) ([]os.FileInfo, error)     { return W.WReaddir(count) }
func (W _net_http_File) Seek(offset int64, whence int) (int64, error) { return W.WSeek(offset, whence) }
func (W _net_http_File) Stat() (os.FileInfo, error)                   { return W.WStat() }

// _net_http_FileSystem is an interface wrapper for FileSystem type
type _net_http_FileSystem struct {
	WOpen func(name string) (http.File, error)
}

func (W _net_http_FileSystem) Open(name string) (http.File, error) { return W.WOpen(name) }

// _net_http_Flusher is an interface wrapper for Flusher type
type _net_http_Flusher struct {
	WFlush func()
}

func (W _net_http_Flusher) Flush() { W.WFlush() }

// _net_http_Handler is an interface wrapper for Handler type
type _net_http_Handler struct {
	WServeHTTP func(a0 http.ResponseWriter, a1 *http.Request)
}

func (W _net_http_Handler) ServeHTTP(a0 http.ResponseWriter, a1 *http.Request) { W.WServeHTTP(a0, a1) }

// _net_http_Hijacker is an interface wrapper for Hijacker type
type _net_http_Hijacker struct {
	WHijack func() (net.Conn, *bufio.ReadWriter, error)
}

func (W _net_http_Hijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) { return W.WHijack() }

// _net_http_Pusher is an interface wrapper for Pusher type
type _net_http_Pusher struct {
	WPush func(target string, opts *http.PushOptions) error
}

func (W _net_http_Pusher) Push(target string, opts *http.PushOptions) error {
	return W.WPush(target, opts)
}

// _net_http_ResponseWriter is an interface wrapper for ResponseWriter type
type _net_http_ResponseWriter struct {
	WHeader      func() http.Header
	WWrite       func(a0 []byte) (int, error)
	WWriteHeader func(statusCode int)
}

func (W _net_http_ResponseWriter) Header() http.Header          { return W.WHeader() }
func (W _net_http_ResponseWriter) Write(a0 []byte) (int, error) { return W.WWrite(a0) }
func (W _net_http_ResponseWriter) WriteHeader(statusCode int)   { W.WWriteHeader(statusCode) }

// _net_http_RoundTripper is an interface wrapper for RoundTripper type
type _net_http_RoundTripper struct {
	WRoundTrip func(a0 *http.Request) (*http.Response, error)
}

func (W _net_http_RoundTripper) RoundTrip(a0 *http.Request) (*http.Response, error) {
	return W.WRoundTrip(a0)
}
