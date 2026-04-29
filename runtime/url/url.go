package url

import (
	"net"
	neturl "net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
)

const (
	punycodeBase        = 36
	punycodeTMin        = 1
	punycodeTMax        = 26
	punycodeSkew        = 38
	punycodeDamp        = 700
	punycodeInitialBias = 72
	punycodeInitialN    = 128
	punycodeDelimiter   = '-'
)

var (
	URLPrototype               *jsvalue.JSValue
	URLConstructor             *jsvalue.JSValue
	URLSearchParamsPrototype   *jsvalue.JSValue
	URLSearchParamsConstructor *jsvalue.JSValue

	stateMu      sync.Mutex
	nextStateID  int
	urlStates    = map[string]*urlState{}
	searchStates = map[string][]searchParamPair{}
)

type urlState struct {
	u *neturl.URL
}

type searchParamPair struct {
	key   string
	value string
}

func jsString(v *jsvalue.JSValue) string {
	if v == nil || v.TypeString() == "undefined" {
		return ""
	}
	return v.String()
}

func jsBool(v *jsvalue.JSValue) bool {
	return v != nil && v.Bool()
}

func setHidden(obj *jsvalue.JSValue, key string, value *jsvalue.JSValue) {
	obj.DefineProperty(key, &jsvalue.PropertyDescriptor{Value: value, Writable: true, Enumerable: false, Configurable: true})
}

func defURLGetter(proto *jsvalue.JSValue, name string, get func(*jsvalue.JSValue) *jsvalue.JSValue) {
	proto.DefineProperty(name, &jsvalue.PropertyDescriptor{Get: get, Enumerable: true, Configurable: true})
}

func defURLAccessor(proto *jsvalue.JSValue, name string, get func(*jsvalue.JSValue) *jsvalue.JSValue, set func(*jsvalue.JSValue, *jsvalue.JSValue)) {
	proto.DefineProperty(name, &jsvalue.PropertyDescriptor{Get: get, Set: set, Enumerable: true, Configurable: true})
}

func newTypeError(message string) *jsvalue.JSValue {
	err := jsvalue.NewObject()
	err.Set("name", jsvalue.NewString("TypeError"))
	err.Set("message", jsvalue.NewString(message))
	return err
}

func invalidURL(input string) *jsvalue.JSValue {
	return newTypeError("Invalid URL: " + input)
}

func urlStateFrom(this *jsvalue.JSValue) *urlState {
	if this == nil {
		return &urlState{u: &neturl.URL{}}
	}
	id := ""
	if idVal := this.Get("_urlID"); idVal != nil && idVal.TypeString() != "undefined" {
		id = idVal.String()
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if id != "" {
		if state := urlStates[id]; state != nil {
			return state
		}
	}
	id = "url" + strconv.Itoa(nextStateID+1)
	nextStateID++
	state := &urlState{u: &neturl.URL{}}
	urlStates[id] = state
	setHidden(this, "_urlID", jsvalue.NewString(id))
	return state
}

func searchPairsFrom(this *jsvalue.JSValue) []searchParamPair {
	if this == nil {
		return nil
	}
	id := ""
	if idVal := this.Get("_searchID"); idVal != nil && idVal.TypeString() != "undefined" {
		id = idVal.String()
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	return append([]searchParamPair(nil), searchStates[id]...)
}

func setSearchPairs(this *jsvalue.JSValue, pairs []searchParamPair) {
	id := ""
	if idVal := this.Get("_searchID"); idVal != nil && idVal.TypeString() != "undefined" {
		id = idVal.String()
	}
	stateMu.Lock()
	if id == "" {
		nextStateID++
		id = "search" + strconv.Itoa(nextStateID)
		setHidden(this, "_searchID", jsvalue.NewString(id))
	}
	searchStates[id] = append([]searchParamPair(nil), pairs...)
	stateMu.Unlock()
	if owner := this.Get("_urlOwner"); owner != nil && owner.TypeString() != "undefined" {
		urlStateFrom(owner).u.RawQuery = encodeSearchPairs(pairs)
	}
}

func parseURL(input string, base *jsvalue.JSValue) (*neturl.URL, bool) {
	if base != nil && base.TypeString() != "undefined" {
		baseURL, err := neturl.Parse(jsString(base))
		if err != nil || baseURL.Scheme == "" {
			return nil, false
		}
		ref, err := neturl.Parse(input)
		if err != nil {
			return nil, false
		}
		return baseURL.ResolveReference(ref), true
	}
	u, err := neturl.Parse(input)
	if err != nil || u.Scheme == "" {
		return nil, false
	}
	return u, true
}

func MakeURL(input string, base *jsvalue.JSValue) *jsvalue.JSValue {
	u, ok := parseURL(input, base)
	if !ok {
		panic(invalidURL(input))
	}
	obj := jsvalue.NewObjectWithPrototype(URLPrototype)
	urlStateFrom(obj).u = u
	setHidden(obj, "searchParams", newURLSearchParamsFromURL(obj))
	return obj
}

func makeSearchParams(pairs []searchParamPair) *jsvalue.JSValue {
	obj := jsvalue.NewObjectWithPrototype(URLSearchParamsPrototype)
	setSearchPairs(obj, append([]searchParamPair(nil), pairs...))
	return obj
}

func newURLSearchParamsFromURL(owner *jsvalue.JSValue) *jsvalue.JSValue {
	obj := jsvalue.NewObjectWithPrototype(URLSearchParamsPrototype)
	setHidden(obj, "_urlOwner", owner)
	setSearchPairs(obj, parseSearchPairs(urlStateFrom(owner).u.RawQuery))
	return obj
}

func syncSearchParams(this *jsvalue.JSValue) {
	params := this.Get("searchParams")
	if params == nil || params.TypeString() == "undefined" {
		return
	}
	setSearchPairs(params, parseSearchPairs(urlStateFrom(this).u.RawQuery))
}

func urlString(u *neturl.URL) string {
	if u == nil {
		return ""
	}
	return u.String()
}

func originString(u *neturl.URL) string {
	if u == nil {
		return "null"
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https", "ftp", "ws", "wss":
		if u.Host == "" {
			return "null"
		}
		return scheme + "://" + u.Host
	case "file":
		return "null"
	default:
		return "null"
	}
}

func setURLString(this *jsvalue.JSValue, raw string) {
	u, ok := parseURL(raw, nil)
	if !ok {
		panic(invalidURL(raw))
	}
	urlStateFrom(this).u = u
	syncSearchParams(this)
}

func protocolOf(u *neturl.URL) string {
	if u == nil || u.Scheme == "" {
		return ":"
	}
	return u.Scheme + ":"
}

func setProtocol(u *neturl.URL, value string) {
	value = strings.TrimSuffix(value, ":")
	if value != "" {
		u.Scheme = strings.ToLower(value)
	}
}

func usernameOf(u *neturl.URL) string {
	if u == nil || u.User == nil {
		return ""
	}
	return u.User.Username()
}

func passwordOf(u *neturl.URL) string {
	if u == nil || u.User == nil {
		return ""
	}
	pw, _ := u.User.Password()
	return pw
}

func setUserInfo(u *neturl.URL, username, password string, hasPassword bool) {
	if username == "" && !hasPassword {
		u.User = nil
		return
	}
	if hasPassword {
		u.User = neturl.UserPassword(username, password)
		return
	}
	u.User = neturl.User(username)
}

func hostOf(u *neturl.URL) string {
	if u == nil {
		return ""
	}
	return u.Host
}

func setHost(u *neturl.URL, value string) {
	u.Host = value
}

func hostnameOf(u *neturl.URL) string {
	if u == nil {
		return ""
	}
	host := u.Hostname()
	if strings.Contains(host, ":") && strings.HasPrefix(u.Host, "[") {
		return "[" + host + "]"
	}
	return host
}

func setHostname(u *neturl.URL, value string) {
	port := u.Port()
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	}
	if port != "" {
		u.Host = net.JoinHostPort(value, port)
		return
	}
	if strings.Contains(value, ":") {
		u.Host = "[" + value + "]"
		return
	}
	u.Host = value
}

func setPort(u *neturl.URL, value string) {
	host := u.Hostname()
	if host == "" {
		return
	}
	if value == "" {
		if strings.Contains(host, ":") {
			u.Host = "[" + host + "]"
		} else {
			u.Host = host
		}
		return
	}
	if strings.Contains(host, ":") {
		u.Host = net.JoinHostPort(host, value)
	} else {
		u.Host = host + ":" + value
	}
}

func pathnameOf(u *neturl.URL) string {
	if u == nil {
		return ""
	}
	if u.EscapedPath() == "" {
		return ""
	}
	return u.EscapedPath()
}

func setPathname(u *neturl.URL, value string) {
	if value == "" {
		u.Path = ""
		u.RawPath = ""
		return
	}
	if !strings.HasPrefix(value, "/") && u.Host != "" {
		value = "/" + value
	}
	parsed, err := neturl.Parse(value)
	if err == nil {
		u.Path = parsed.Path
		u.RawPath = parsed.RawPath
		return
	}
	u.Path = value
	u.RawPath = ""
}

func searchOf(u *neturl.URL) string {
	if u == nil || u.RawQuery == "" {
		return ""
	}
	return "?" + u.RawQuery
}

func setSearch(u *neturl.URL, value string) {
	value = strings.TrimPrefix(value, "?")
	u.RawQuery = value
}

func hashOf(u *neturl.URL) string {
	if u == nil || u.Fragment == "" {
		return ""
	}
	return "#" + u.EscapedFragment()
}

func setHash(u *neturl.URL, value string) {
	value = strings.TrimPrefix(value, "#")
	frag, err := neturl.PathUnescape(value)
	if err != nil {
		u.Fragment = value
		u.RawFragment = ""
		return
	}
	u.Fragment = frag
	u.RawFragment = value
}

func canParse(input string, base *jsvalue.JSValue) bool {
	_, ok := parseURL(input, base)
	return ok
}

func parseSearchPairs(raw string) []searchParamPair {
	raw = strings.TrimPrefix(raw, "?")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "&")
	pairs := make([]searchParamPair, 0, len(parts))
	for _, part := range parts {
		key, val, found := strings.Cut(part, "=")
		if !found {
			val = ""
		}
		key = strings.ReplaceAll(key, "+", " ")
		val = strings.ReplaceAll(val, "+", " ")
		k, err := neturl.QueryUnescape(key)
		if err != nil {
			k = key
		}
		v, err := neturl.QueryUnescape(val)
		if err != nil {
			v = val
		}
		pairs = append(pairs, searchParamPair{key: k, value: v})
	}
	return pairs
}

func encodeSearchPairs(pairs []searchParamPair) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := make([]string, len(pairs))
	for i, pair := range pairs {
		parts[i] = neturl.QueryEscape(pair.key) + "=" + neturl.QueryEscape(pair.value)
	}
	return strings.Join(parts, "&")
}

func searchPairsFromInit(init *jsvalue.JSValue) []searchParamPair {
	if init == nil || init.TypeString() == "undefined" {
		return nil
	}
	if init.IsArray() {
		arr := init.Array()
		pairs := make([]searchParamPair, 0, len(arr))
		for _, item := range arr {
			if item == nil || !item.IsArray() || item.Len() < 2 {
				continue
			}
			pairs = append(pairs, searchParamPair{key: item.Index(0).String(), value: item.Index(1).String()})
		}
		return pairs
	}
	if init.TypeString() == "object" {
		if pairs := searchPairsFrom(init); pairs != nil {
			return append([]searchParamPair(nil), pairs...)
		}
		keys := init.OwnKeys()
		pairs := make([]searchParamPair, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, searchParamPair{key: key, value: init.Get(key).String()})
		}
		return pairs
	}
	return parseSearchPairs(init.String())
}

func searchParamsAppend(this *jsvalue.JSValue, key, value string) {
	pairs := append(searchPairsFrom(this), searchParamPair{key: key, value: value})
	setSearchPairs(this, pairs)
}

func searchParamsDelete(this *jsvalue.JSValue, key string, value *jsvalue.JSValue) {
	pairs := searchPairsFrom(this)
	out := pairs[:0]
	hasValue := value != nil && value.TypeString() != "undefined"
	want := jsString(value)
	for _, pair := range pairs {
		if pair.key == key && (!hasValue || pair.value == want) {
			continue
		}
		out = append(out, pair)
	}
	setSearchPairs(this, append([]searchParamPair(nil), out...))
}

func searchParamsGet(this *jsvalue.JSValue, key string) *jsvalue.JSValue {
	for _, pair := range searchPairsFrom(this) {
		if pair.key == key {
			return jsvalue.NewString(pair.value)
		}
	}
	return jsvalue.NewNull()
}

func searchParamsGetAll(this *jsvalue.JSValue, key string) *jsvalue.JSValue {
	out := []*jsvalue.JSValue{}
	for _, pair := range searchPairsFrom(this) {
		if pair.key == key {
			out = append(out, jsvalue.NewString(pair.value))
		}
	}
	return jsvalue.NewArray(out...)
}

func searchParamsHas(this *jsvalue.JSValue, key string, value *jsvalue.JSValue) bool {
	hasValue := value != nil && value.TypeString() != "undefined"
	want := jsString(value)
	for _, pair := range searchPairsFrom(this) {
		if pair.key == key && (!hasValue || pair.value == want) {
			return true
		}
	}
	return false
}

func searchParamsSet(this *jsvalue.JSValue, key, value string) {
	pairs := searchPairsFrom(this)
	out := make([]searchParamPair, 0, len(pairs)+1)
	seen := false
	for _, pair := range pairs {
		if pair.key != key {
			out = append(out, pair)
			continue
		}
		if !seen {
			out = append(out, searchParamPair{key: key, value: value})
			seen = true
		}
	}
	if !seen {
		out = append(out, searchParamPair{key: key, value: value})
	}
	setSearchPairs(this, out)
}

func searchParamsSort(this *jsvalue.JSValue) {
	pairs := append([]searchParamPair(nil), searchPairsFrom(this)...)
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	setSearchPairs(this, pairs)
}

func searchParamsArray(this *jsvalue.JSValue, mode string) *jsvalue.JSValue {
	pairs := searchPairsFrom(this)
	out := make([]*jsvalue.JSValue, 0, len(pairs))
	for _, pair := range pairs {
		switch mode {
		case "keys":
			out = append(out, jsvalue.NewString(pair.key))
		case "values":
			out = append(out, jsvalue.NewString(pair.value))
		default:
			out = append(out, jsvalue.NewArray(jsvalue.NewString(pair.key), jsvalue.NewString(pair.value)))
		}
	}
	return jsvalue.NewArray(out...)
}

func URLSearchParamsString(this *jsvalue.JSValue) string {
	return encodeSearchPairs(searchPairsFrom(this))
}

func FileURLToPath(input *jsvalue.JSValue) *jsvalue.JSValue {
	s := jsString(input)
	if input != nil {
		if id := input.Get("_urlID"); id != nil && id.TypeString() != "undefined" {
			s = urlString(urlStateFrom(input).u)
		}
	}
	u, err := neturl.Parse(s)
	if err != nil || u.Scheme != "file" {
		panic(newTypeError("The URL must be of scheme file"))
	}
	if u.Host != "" && u.Host != "localhost" {
		if runtime.GOOS == "windows" {
			return jsvalue.NewString(`\\` + u.Host + filepath.FromSlash(u.Path))
		}
		panic(newTypeError("File URL host must be localhost or empty on this platform"))
	}
	p, err := neturl.PathUnescape(u.Path)
	if err != nil {
		p = u.Path
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/") && len(p) >= 3 && p[2] == ':' {
		p = p[1:]
	}
	return jsvalue.NewString(filepath.FromSlash(p))
}

func PathToFileURL(input *jsvalue.JSValue) *jsvalue.JSValue {
	p := jsString(input)
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err == nil {
			p = abs
		}
	}
	slash := filepath.ToSlash(p)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := &neturl.URL{Scheme: "file", Path: slash}
	return MakeURL(u.String(), nil)
}

func domainToASCIIString(domain string) string {
	if domain == "" {
		return ""
	}
	labels := strings.Split(domain, ".")
	for i, label := range labels {
		if label == "" {
			continue
		}
		ascii := true
		for _, r := range label {
			if r >= utf8.RuneSelf {
				ascii = false
				break
			}
		}
		if ascii {
			labels[i] = strings.ToLower(label)
			continue
		}
		enc, ok := punycodeEncode([]rune(strings.ToLower(label)))
		if !ok {
			return ""
		}
		labels[i] = "xn--" + enc
	}
	return strings.Join(labels, ".")
}

func domainToUnicodeString(domain string) string {
	if domain == "" {
		return ""
	}
	labels := strings.Split(domain, ".")
	for i, label := range labels {
		if strings.HasPrefix(strings.ToLower(label), "xn--") {
			dec, ok := punycodeDecode(label[4:])
			if ok {
				labels[i] = string(dec)
			}
		}
	}
	return strings.Join(labels, ".")
}

func DomainToASCII(input *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewString(domainToASCIIString(jsString(input)))
}

func DomainToUnicode(input *jsvalue.JSValue) *jsvalue.JSValue {
	return jsvalue.NewString(domainToUnicodeString(jsString(input)))
}

func Format(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) == 0 || args[0] == nil {
		return jsvalue.NewString("")
	}
	if id := args[0].Get("_urlID"); id != nil && id.TypeString() != "undefined" {
		return jsvalue.NewString(urlString(urlStateFrom(args[0]).u))
	}
	if args[0].TypeString() == "object" {
		return jsvalue.NewString(formatLegacyObject(args[0]))
	}
	return jsvalue.NewString(args[0].String())
}

func formatLegacyObject(obj *jsvalue.JSValue) string {
	protocol := jsString(obj.Get("protocol"))
	slashes := jsBool(obj.Get("slashes"))
	auth := jsString(obj.Get("auth"))
	host := jsString(obj.Get("host"))
	if host == "" {
		hostname := jsString(obj.Get("hostname"))
		port := jsString(obj.Get("port"))
		host = hostname
		if port != "" {
			host += ":" + port
		}
	}
	pathname := jsString(obj.Get("pathname"))
	search := jsString(obj.Get("search"))
	query := obj.Get("query")
	hash := jsString(obj.Get("hash"))
	if protocol != "" && !strings.HasSuffix(protocol, ":") {
		protocol += ":"
	}
	out := protocol
	if slashes || host != "" {
		out += "//"
		if auth != "" {
			out += neturl.QueryEscape(auth) + "@"
		}
		out += host
	}
	if pathname != "" {
		if host != "" && !strings.HasPrefix(pathname, "/") {
			out += "/"
		}
		out += pathname
	}
	if search != "" {
		if !strings.HasPrefix(search, "?") {
			out += "?"
		}
		out += strings.TrimPrefix(search, "?")
	} else if query != nil && query.TypeString() != "undefined" {
		if query.TypeString() == "object" {
			out += "?" + encodeSearchPairs(searchPairsFromInit(query))
		} else if query.String() != "" {
			out += "?" + query.String()
		}
	}
	if hash != "" {
		if !strings.HasPrefix(hash, "#") {
			out += "#"
		}
		out += strings.TrimPrefix(hash, "#")
	}
	return out
}

func Parse(input *jsvalue.JSValue, parseQueryString *jsvalue.JSValue, slashesDenoteHost *jsvalue.JSValue) *jsvalue.JSValue {
	raw := jsString(input)
	parseInput := raw
	if jsBool(slashesDenoteHost) && strings.HasPrefix(parseInput, "//") {
		parseInput = "http:" + parseInput
	}
	u, err := neturl.Parse(parseInput)
	if err != nil {
		return jsvalue.NewObject()
	}
	protocol := ""
	if u.Scheme != "" {
		protocol = u.Scheme + ":"
	}
	auth := ""
	if u.User != nil {
		auth = u.User.String()
	}
	pathname := u.EscapedPath()
	search := ""
	if u.RawQuery != "" {
		search = "?" + u.RawQuery
	}
	queryVal := jsvalue.NewString(u.RawQuery)
	if jsBool(parseQueryString) {
		queryVal = jsvalue.NewObject()
		for _, pair := range parseSearchPairs(u.RawQuery) {
			existing := queryVal.Get(pair.key)
			if existing != nil && existing.TypeString() != "undefined" {
				if existing.IsArray() {
					existing.MethodCall("push", jsvalue.NewString(pair.value))
				} else {
					queryVal.Set(pair.key, jsvalue.NewArray(existing, jsvalue.NewString(pair.value)))
				}
			} else {
				queryVal.Set(pair.key, jsvalue.NewString(pair.value))
			}
		}
	}
	obj := jsvalue.ObjectFrom(
		"href", jsvalue.NewString(raw),
		"protocol", jsvalue.NewString(protocol),
		"slashes", jsvalue.NewBool(strings.HasPrefix(raw, "//") || u.Host != ""),
		"auth", jsvalue.NewString(auth),
		"host", jsvalue.NewString(u.Host),
		"port", jsvalue.NewString(u.Port()),
		"hostname", jsvalue.NewString(u.Hostname()),
		"hash", jsvalue.NewString(hashOf(u)),
		"search", jsvalue.NewString(search),
		"query", queryVal,
		"pathname", jsvalue.NewString(pathname),
		"path", jsvalue.NewString(pathname+search),
	)
	return obj
}

func Resolve(from, to *jsvalue.JSValue) *jsvalue.JSValue {
	base, err := neturl.Parse(jsString(from))
	if err != nil {
		return jsvalue.NewString(jsString(to))
	}
	ref, err := neturl.Parse(jsString(to))
	if err != nil {
		return jsvalue.NewString(jsString(to))
	}
	return jsvalue.NewString(base.ResolveReference(ref).String())
}

func URLToHttpOptions(input *jsvalue.JSValue) *jsvalue.JSValue {
	state := urlStateFrom(input)
	u := state.u
	pathValue := pathnameOf(u)
	if q := searchOf(u); q != "" {
		pathValue += q
	}
	obj := jsvalue.ObjectFrom(
		"protocol", jsvalue.NewString(protocolOf(u)),
		"hostname", jsvalue.NewString(u.Hostname()),
		"hash", jsvalue.NewString(hashOf(u)),
		"search", jsvalue.NewString(searchOf(u)),
		"pathname", jsvalue.NewString(pathnameOf(u)),
		"path", jsvalue.NewString(pathValue),
		"href", jsvalue.NewString(urlString(u)),
	)
	if port := u.Port(); port != "" {
		obj.Set("port", jsvalue.NewNumber(float64(mustAtoi(port))))
	}
	if u.User != nil {
		obj.Set("auth", jsvalue.NewString(u.User.String()))
	}
	return obj
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func punycodeEncode(input []rune) (string, bool) {
	var out []rune
	for _, r := range input {
		if r < 0x80 {
			out = append(out, r)
		}
	}
	basicLen := len(out)
	if basicLen > 0 {
		out = append(out, punycodeDelimiter)
	}
	n := punycodeInitialN
	delta := 0
	bias := punycodeInitialBias
	h := basicLen
	for h < len(input) {
		m := int(^uint(0) >> 1)
		for _, r := range input {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		if m-n > (int(^uint(0)>>1)-delta)/(h+1) {
			return "", false
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range input {
			if int(r) < n {
				delta++
				if delta == 0 {
					return "", false
				}
			}
			if int(r) == n {
				q := delta
				for k := punycodeBase; ; k += punycodeBase {
					t := punycodeThreshold(k, bias)
					if q < t {
						break
					}
					out = append(out, encodeDigit(t+(q-t)%(punycodeBase-t)))
					q = (q - t) / (punycodeBase - t)
				}
				out = append(out, encodeDigit(q))
				bias = adaptPunycode(delta, h+1, h == basicLen)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return string(out), true
}

func punycodeDecode(input string) ([]rune, bool) {
	n := punycodeInitialN
	i := 0
	bias := punycodeInitialBias
	out := []rune{}
	if d := strings.LastIndexByte(input, punycodeDelimiter); d >= 0 {
		for _, r := range input[:d] {
			if r >= 0x80 {
				return nil, false
			}
			out = append(out, r)
		}
		input = input[d+1:]
	}
	for len(input) > 0 {
		oldi := i
		w := 1
		for k := punycodeBase; ; k += punycodeBase {
			if len(input) == 0 {
				return nil, false
			}
			r := rune(input[0])
			input = input[1:]
			digit := decodeDigit(r)
			if digit < 0 {
				return nil, false
			}
			i += digit * w
			t := punycodeThreshold(k, bias)
			if digit < t {
				break
			}
			w *= punycodeBase - t
		}
		bias = adaptPunycode(i-oldi, len(out)+1, oldi == 0)
		n += i / (len(out) + 1)
		i %= len(out) + 1
		out = append(out, 0)
		copy(out[i+1:], out[i:])
		out[i] = rune(n)
		i++
	}
	return out, true
}

func punycodeThreshold(k, bias int) int {
	if k <= bias+punycodeTMin {
		return punycodeTMin
	}
	if k >= bias+punycodeTMax {
		return punycodeTMax
	}
	return k - bias
}

func adaptPunycode(delta, numPoints int, first bool) int {
	if first {
		delta /= punycodeDamp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := 0
	for delta > ((punycodeBase-punycodeTMin)*punycodeTMax)/2 {
		delta /= punycodeBase - punycodeTMin
		k += punycodeBase
	}
	return k + (punycodeBase-punycodeTMin+1)*delta/(delta+punycodeSkew)
}

func encodeDigit(d int) rune {
	if d < 26 {
		return rune('a' + d)
	}
	return rune('0' + d - 26)
}

func decodeDigit(r rune) int {
	switch {
	case r >= 'a' && r <= 'z':
		return int(r - 'a')
	case r >= 'A' && r <= 'Z':
		return int(r - 'A')
	case r >= '0' && r <= '9':
		return int(r-'0') + 26
	default:
		return -1
	}
}
