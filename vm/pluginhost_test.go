package vm

// The tests of package vm call the plugins of the measured catalog as
// internal/pluginhost does (it imports package vm, so they cannot use it).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func createConfig(p *Plugin) (*Object, error) {
	res, err := p.Call(context.Background(), "CreateConfig")
	if err != nil {
		return nil, err
	}
	return res[0], nil
}

func newHandler(ctx context.Context, p *Plugin, next http.Handler, cfg *Object, name string) (http.Handler, error) {
	res, err := p.Call(ctx, "New", ctx, next, cfg, name)
	if err != nil {
		return nil, err
	}
	if err := resultErr(res[1]); err != nil {
		return nil, err
	}
	v, err := res[0].Interface()
	if err != nil {
		return nil, err
	}
	h, ok := v.(http.Handler)
	if !ok {
		return nil, fmt.Errorf("the New function returns %T, an http.Handler is expected", v)
	}
	return h, nil
}

func newProvider(ctx context.Context, p *Plugin, cfg *Object, name string) (*Object, error) {
	res, err := p.Call(ctx, "New", ctx, cfg, name)
	if err != nil {
		return nil, err
	}
	if err := resultErr(res[1]); err != nil {
		return nil, err
	}
	return res[0], nil
}

func callMethod(o *Object, name string, args ...any) ([]any, error) {
	res, err := o.Call(context.Background(), name, args...)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(res))
	for i, r := range res {
		if out[i], err = r.Interface(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func resultErr(r *Object) error {
	v, err := r.Interface()
	if err != nil || v == nil {
		return err
	}
	e, ok := v.(error)
	if !ok {
		return errors.New("the error result is not an error")
	}
	return e
}
