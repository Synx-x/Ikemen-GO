//go:build js

package main

import (
	"errors"
	"fmt"
	"strings"
	"syscall/js"
)

// FetchAsset downloads an asset relative to the harness root via fetch().
// Strips leading "./" and prepends "/assets/". Synchronous-feeling via
// channel + Promise.then/catch callbacks.
//
// Engine path examples:
//   "./data/system.def"        -> GET /assets/data/system.def
//   "chars/kfm/kfm.def"        -> GET /assets/chars/kfm/kfm.def
//   "/assets/font/font.def"    -> GET /assets/font/font.def (already prefixed)
func FetchAsset(path string) ([]byte, error) {
	path = strings.TrimPrefix(path, "./")
	url := path
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "http") {
		url = "/assets/" + path
	}

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)

	var thenFunc, catchFunc, arrBufThen js.Func
	cleanup := func() {
		thenFunc.Release()
		catchFunc.Release()
		arrBufThen.Release()
	}

	arrBufThen = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		uint8 := js.Global().Get("Uint8Array").New(args[0])
		n := uint8.Get("length").Int()
		buf := make([]byte, n)
		js.CopyBytesToGo(buf, uint8)
		ch <- result{data: buf, err: nil}
		return nil
	})

	thenFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resp := args[0]
		ok := resp.Get("ok").Bool()
		status := resp.Get("status").Int()
		if !ok {
			ch <- result{err: fmt.Errorf("fetch %s: HTTP %d", url, status)}
			return nil
		}
		resp.Call("arrayBuffer").Call("then", arrBufThen).Call("catch", catchFunc)
		return nil
	})

	catchFunc = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		msg := "unknown"
		if len(args) > 0 && args[0].Truthy() {
			msg = args[0].Get("message").String()
		}
		ch <- result{err: fmt.Errorf("fetch %s: %s", url, msg)}
		return nil
	})

	js.Global().Call("fetch", url).Call("then", thenFunc).Call("catch", catchFunc)

	r := <-ch
	cleanup()
	if r.err != nil {
		return nil, r.err
	}
	if r.data == nil {
		return nil, errors.New("fetch returned no data")
	}
	return r.data, nil
}
