package url

import jsvalue "github.com/nnstd/gun/runtime/builtin"

func init() {
	URLPrototype = jsvalue.NewObject()
	URLSearchParamsPrototype = jsvalue.NewObject()

	URLConstructor = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		input := ""
		if len(args) > 0 {
			input = args[0].String()
		}
		var base *jsvalue.JSValue
		if len(args) > 1 {
			base = args[1]
		}
		parsed, ok := parseURL(input, base)
		if !ok {
			panic(invalidURL(input))
		}
		this.SetPrototype(URLPrototype)
		urlStateFrom(this).u = parsed
		setHidden(this, "searchParams", newURLSearchParamsFromURL(this))
		return nil
	}, nil)
	URLConstructor.Set("prototype", URLPrototype)
	URLConstructor.Set("canParse", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		input := ""
		if len(args) > 0 {
			input = args[0].String()
		}
		var base *jsvalue.JSValue
		if len(args) > 1 {
			base = args[1]
		}
		return jsvalue.NewBool(canParse(input, base))
	}))
	URLConstructor.Set("parse", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		input := ""
		if len(args) > 0 {
			input = args[0].String()
		}
		var base *jsvalue.JSValue
		if len(args) > 1 {
			base = args[1]
		}
		if !canParse(input, base) {
			return jsvalue.NewNull()
		}
		return MakeURL(input, base)
	}))

	URLSearchParamsConstructor = jsvalue.NewClass(func(this *jsvalue.JSValue, args ...*jsvalue.JSValue) *jsvalue.JSValue {
		this.SetPrototype(URLSearchParamsPrototype)
		var init *jsvalue.JSValue
		if len(args) > 0 {
			init = args[0]
		}
		setSearchPairs(this, searchPairsFromInit(init))
		return nil
	}, nil)
	URLSearchParamsConstructor.Set("prototype", URLSearchParamsPrototype)

	installURLPrototype()
	installURLSearchParamsPrototype()
	AsJSValue = makeModuleExports()
	jsvalue.RegisterGlobal("URL", URLConstructor)
	jsvalue.RegisterGlobal("URLSearchParams", URLSearchParamsConstructor)
}

func installURLPrototype() {
	defURLAccessor(URLPrototype, "href", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(urlString(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) { setURLString(this, value.String()) })
	defURLGetter(URLPrototype, "origin", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(originString(urlStateFrom(this).u))
	})
	defURLAccessor(URLPrototype, "protocol", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(protocolOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) { setProtocol(urlStateFrom(this).u, value.String()) })
	defURLAccessor(URLPrototype, "username", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(usernameOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) {
		u := urlStateFrom(this).u
		_, hasPassword := u.User.Password()
		setUserInfo(u, value.String(), passwordOf(u), hasPassword)
	})
	defURLAccessor(URLPrototype, "password", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(passwordOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) {
		u := urlStateFrom(this).u
		setUserInfo(u, usernameOf(u), value.String(), true)
	})
	defURLAccessor(URLPrototype, "host", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(hostOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) { setHost(urlStateFrom(this).u, value.String()) })
	defURLAccessor(URLPrototype, "hostname", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(hostnameOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) { setHostname(urlStateFrom(this).u, value.String()) })
	defURLAccessor(URLPrototype, "port", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(urlStateFrom(this).u.Port())
	}, func(this, value *jsvalue.JSValue) { setPort(urlStateFrom(this).u, value.String()) })
	defURLAccessor(URLPrototype, "pathname", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(pathnameOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) { setPathname(urlStateFrom(this).u, value.String()) })
	defURLAccessor(URLPrototype, "search", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(searchOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) {
		setSearch(urlStateFrom(this).u, value.String())
		syncSearchParams(this)
	})
	defURLAccessor(URLPrototype, "hash", func(this *jsvalue.JSValue) *jsvalue.JSValue {
		return jsvalue.NewString(hashOf(urlStateFrom(this).u))
	}, func(this, value *jsvalue.JSValue) { setHash(urlStateFrom(this).u, value.String()) })
	URLPrototype.Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) == 0 {
			return jsvalue.NewString("")
		}
		return jsvalue.NewString(urlString(urlStateFrom(args[0]).u))
	}).MarkAsMethod())
	URLPrototype.Set("toJSON", URLPrototype.Get("toString"))
}

func installURLSearchParamsPrototype() {
	URLSearchParamsPrototype.Set("append", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 3 {
			searchParamsAppend(args[0], args[1].String(), args[2].String())
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("delete", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			var value *jsvalue.JSValue
			if len(args) >= 3 {
				value = args[2]
			}
			searchParamsDelete(args[0], args[1].String(), value)
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("get", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return searchParamsGet(args[0], args[1].String())
		}
		return jsvalue.NewNull()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("getAll", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return searchParamsGetAll(args[0], args[1].String())
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("has", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			var value *jsvalue.JSValue
			if len(args) >= 3 {
				value = args[2]
			}
			return jsvalue.NewBool(searchParamsHas(args[0], args[1].String(), value))
		}
		return jsvalue.NewBool(false)
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("set", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 3 {
			searchParamsSet(args[0], args[1].String(), args[2].String())
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("sort", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			searchParamsSort(args[0])
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("entries", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return searchParamsArray(args[0], "entries")
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("keys", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return searchParamsArray(args[0], "keys")
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("values", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return searchParamsArray(args[0], "values")
		}
		return jsvalue.NewArray()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("forEach", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 && args[1] != nil {
			thisArg := jsvalue.NewUndefined()
			if len(args) >= 3 {
				thisArg = args[2]
			}
			for _, pair := range searchPairsFrom(args[0]) {
				args[1].Call(jsvalue.NewString(pair.value), jsvalue.NewString(pair.key), args[0], thisArg)
			}
		}
		return jsvalue.NewUndefined()
	}).MarkAsMethod())
	URLSearchParamsPrototype.Set("toString", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 1 {
			return jsvalue.NewString(URLSearchParamsString(args[0]))
		}
		return jsvalue.NewString("")
	}).MarkAsMethod())
}

func makeModuleExports() *jsvalue.JSValue {
	obj := jsvalue.NewObject()
	obj.Set("URL", URLConstructor)
	obj.Set("URLSearchParams", URLSearchParamsConstructor)
	obj.Set("domainToASCII", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return DomainToASCII(args[0])
		}
		return jsvalue.NewString("")
	}))
	obj.Set("domainToUnicode", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return DomainToUnicode(args[0])
		}
		return jsvalue.NewString("")
	}))
	obj.Set("fileURLToPath", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return FileURLToPath(args[0])
		}
		return jsvalue.NewString("")
	}))
	obj.Set("pathToFileURL", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return PathToFileURL(args[0])
		}
		return PathToFileURL(jsvalue.NewString(""))
	}))
	obj.Set("format", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue { return Format(args...) }))
	obj.Set("parse", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		var input, parseQS, slashes *jsvalue.JSValue
		if len(args) > 0 {
			input = args[0]
		}
		if len(args) > 1 {
			parseQS = args[1]
		}
		if len(args) > 2 {
			slashes = args[2]
		}
		return Parse(input, parseQS, slashes)
	}))
	obj.Set("resolve", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) >= 2 {
			return Resolve(args[0], args[1])
		}
		return jsvalue.NewString("")
	}))
	obj.Set("urlToHttpOptions", jsvalue.NewFunction(func(args ...*jsvalue.JSValue) *jsvalue.JSValue {
		if len(args) > 0 {
			return URLToHttpOptions(args[0])
		}
		return jsvalue.NewObject()
	}))
	return obj
}

// AsJSValue returns the url module as a JSValue object with all exports.
var AsJSValue *jsvalue.JSValue
