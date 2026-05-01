package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"

	jsvalue "github.com/nnstd/gun/runtime/builtin"
	"github.com/nnstd/gun/runtime/buffer"
)

// ---------------------------------------------------------------------------
// generatePrimeSync — synchronous prime generation
// ---------------------------------------------------------------------------

func generatePrimeSyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 || args[0] == nil {
		panic(errInvalidArgType("a number (size in bits)"))
	}
	size := int(args[0].Number())
	if size < 2 {
		panic(errOutOfRange("The value of \"size\" is out of range. It must be >= 2"))
	}

	var addVal *big.Int
	var remVal *big.Int
	safe := false

	if len(args) > 1 && args[1] != nil && args[1].TypeString() == "object" {
		opts := args[1]
		if v := opts.Get("safe"); v != nil && v.TypeString() != "undefined" {
			safe = v.Bool()
		}
		if v := opts.Get("add"); v != nil && v.TypeString() != "undefined" {
			addBytes := inputBytes(v)
			if len(addBytes) > 0 {
				addVal = new(big.Int).SetBytes(addBytes)
			}
		}
		if v := opts.Get("rem"); v != nil && v.TypeString() != "undefined" {
			remBytes := inputBytes(v)
			if len(remBytes) > 0 {
				remVal = new(big.Int).SetBytes(remBytes)
			}
		}
	}

	// For safe primes, we generate p where (p-1)/2 is also prime.
	if safe {
		// Generate a prime q first, then p = 2q + 1
		qBits := size - 1
		if qBits < 2 {
			qBits = 2
		}
		q, err := generatePrimeWithConstraints(qBits, addVal, remVal)
		if err != nil {
			panic(errOperationFailed("Failed to generate safe prime: " + err.Error()))
		}
		// p = 2*q + 1
		p := new(big.Int).Mul(q, big.NewInt(2))
		p.Add(p, big.NewInt(1))
		return buffer.NewBufferFromBytes(p.Bytes())
	}

	prime, err := generatePrimeWithConstraints(size, addVal, remVal)
	if err != nil {
		panic(errOperationFailed("Failed to generate prime: " + err.Error()))
	}

	return buffer.NewBufferFromBytes(prime.Bytes())
}

// generatePrimeWithConstraints generates a random prime of the given bit size,
// optionally satisfying prime = add*k + rem for some integer k.
func generatePrimeWithConstraints(size int, add, rem *big.Int) (*big.Int, error) {
	if add == nil || add.Sign() == 0 {
		// No constraints — use math/rand directly
		return rand.Prime(rand.Reader, size)
	}

	// Constrained generation: prime = add*k + rem
	// We need add >= 2 and 0 <= rem < add
	if add.Sign() <= 0 {
		return nil, fmt.Errorf("add must be a positive integer")
	}

	if rem != nil && rem.Cmp(add) >= 0 {
		return nil, fmt.Errorf("rem must be less than add")
	}

	// k must be at least 1 (so prime >= add + rem > add)
	// We want prime to be approximately 2^size, so k ≈ 2^size / add
	one := big.NewInt(1)
	// Compute minimum k: prime = add*k + rem >= 2^(size-1)
	minPrime := new(big.Int).Lsh(one, uint(size-1))
	k := new(big.Int)
	if rem != nil {
		k.Sub(minPrime, rem)
	} else {
		k.Set(minPrime)
	}
	k.Sub(k, one)
	k.Div(k, add)
	if k.Sign() <= 0 {
		k.Set(one)
	}
	k.Add(k, one)

	// Try random values of k until we find a prime
	for {
		candidate := new(big.Int).Mul(add, k)
		if rem != nil {
			candidate.Add(candidate, rem)
		}
		// Add some randomness to k
		randK, err := rand.Int(rand.Reader, big.NewInt(1000))
		if err != nil {
			return nil, err
		}
		k.Add(k, randK)
		if candidate.BitLen() > size {
			// Reset k
			k.Div(minPrime, add)
			if k.Sign() <= 0 {
				k.Set(one)
			}
			continue
		}
		if candidate.BitLen() == size && candidate.ProbablyPrime(20) {
			return candidate, nil
		}
	}
}

// ---------------------------------------------------------------------------
// generatePrime — asynchronous prime generation
// ---------------------------------------------------------------------------

func generatePrimeJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 || args[0] == nil {
		panic(errInvalidArgType("a number (size in bits)"))
	}

	// Check for callback variant: generatePrime(size, options, callback)
	cbIdx := -1
	for i := len(args) - 1; i >= 2; i-- {
		if args[i] != nil && args[i].TypeString() == "function" {
			cbIdx = i
			break
		}
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		syncArgs := make([]*jsvalue.JSValue, len(args))
		copy(syncArgs, args)
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			result := generatePrimeSyncJS(syncArgs...)
			return result, nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(generatePrimeSyncJS(args...))
}

// ---------------------------------------------------------------------------
// checkPrimeSync — synchronous prime check
// ---------------------------------------------------------------------------

func checkPrimeSyncJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 || args[0] == nil {
		panic(errInvalidArgType("a Buffer, TypedArray, or BigInt"))
	}

	candidateBytes := inputBytes(args[0])
	if len(candidateBytes) == 0 {
		panic(errInvalidArgType("a Buffer, TypedArray, or BigInt"))
	}

	candidate := new(big.Int).SetBytes(candidateBytes)

	checks := 16 // default number of Miller-Rabin rounds
	if len(args) > 1 && args[1] != nil && args[1].TypeString() == "object" {
		opts := args[1]
		if v := opts.Get("checks"); v != nil && v.TypeString() != "undefined" {
			checks = int(v.Number())
		}
	}

	if checks < 1 {
		checks = 1
	}

	isPrime := candidate.ProbablyPrime(checks)
	return jsvalue.NewBool(isPrime)
}

// ---------------------------------------------------------------------------
// checkPrime — asynchronous prime check
// ---------------------------------------------------------------------------

func checkPrimeJS(args ...*jsvalue.JSValue) *jsvalue.JSValue {
	if len(args) < 1 || args[0] == nil {
		panic(errInvalidArgType("a Buffer, TypedArray, or BigInt"))
	}

	// Check for callback variant: checkPrime(candidate, options, callback)
	cbIdx := -1
	for i := len(args) - 1; i >= 2; i-- {
		if args[i] != nil && args[i].TypeString() == "function" {
			cbIdx = i
			break
		}
	}

	if cbIdx >= 0 {
		cb := args[cbIdx]
		syncArgs := make([]*jsvalue.JSValue, len(args))
		copy(syncArgs, args)
		asyncCrypto(func() (*jsvalue.JSValue, error) {
			result := checkPrimeSyncJS(syncArgs...)
			return result, nil
		}, cb)
		return jsvalue.NewUndefined()
	}

	return resolvePromise(checkPrimeSyncJS(args...))
}
