package luaplug

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// state runs a Lua 5.1 script with gopher-lua: tables, closures,
// coroutines, string patterns and Go functions called from Lua.
type state struct {
	mu sync.Mutex
	L  *lua.LState
}

const script = `
local counts = {}
local function words(s)
  local t = {}
  for w in string.gmatch(s, "[^/]+") do t[#t+1] = w end
  return t
end
local gen = coroutine.wrap(function()
  local i = 0
  while true do i = i + 1; coroutine.yield(i * i) end
end)
function handle(req)
  counts[req.method] = (counts[req.method] or 0) + 1
  local parts = words(req.path)
  local hdrs = {}
  for k, _ in pairs(req.headers) do hdrs[#hdrs+1] = string.lower(k) end
  table.sort(hdrs)
  local ok, err = pcall(function() if req.path:find("admin") then error("admin denied") end end)
  local verdict = ok and "allow" or ("deny:" .. tostring(err):match("admin denied"))
  return string.format("%s parts=%d first=%s count=%d sq=%d hdrs=%s gosum=%d", verdict, #parts,
    parts[1] or "-", counts[req.method], gen(), table.concat(hdrs, ","), gosum(1, 2, #parts))
end
`

func setup(_ context.Context) (*state, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	L.SetGlobal("gosum", L.NewFunction(func(L *lua.LState) int {
		n := 0
		for i := 1; i <= L.GetTop(); i++ {
			n += L.CheckInt(i)
		}
		L.Push(lua.LNumber(n))
		return 1
	}))
	if err := L.DoString(script); err != nil {
		return nil, err
	}
	return &state{L: L}, nil
}

func (s *state) process(req *http.Request) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.L.NewTable()
	t.RawSetString("method", lua.LString(req.Method))
	t.RawSetString("path", lua.LString(req.URL.Path))
	h := s.L.NewTable()
	keys := make([]string, 0, len(req.Header))
	for k := range req.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.RawSetString(k, lua.LString(strings.Join(req.Header[k], ",")))
	}
	t.RawSetString("headers", h)
	if err := s.L.CallByParam(lua.P{Fn: s.L.GetGlobal("handle"), NRet: 1, Protect: true}, t); err != nil {
		return "", err
	}
	ret := s.L.Get(-1)
	s.L.Pop(1)
	return fmt.Sprint(ret.String()), nil
}
