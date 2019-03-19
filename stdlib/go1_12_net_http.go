// +build go1.12, !go1.13

package stdlib

// Code generated by 'goexports net/http'. DO NOT EDIT.

import (
	"net/http"
	"reflect"
)

func init() {
	Value["net/http"] = map[string]reflect.Value{
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
	}
}
