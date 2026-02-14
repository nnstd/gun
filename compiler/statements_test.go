package compiler

import (
	"strings"
	"testing"
)

func TestIfStatement(t *testing.T) {
	ts := `function check(x: number): string {
		if (x > 0) { return "pos"; } else { return "neg"; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "if x > 0")
	assertContains(t, out, `return "pos"`)
	assertContains(t, out, `return "neg"`)
}

func TestForOfLoop(t *testing.T) {
	ts := `function sum(items: number[]): number {
		let total = 0;
		for (const item of items) { total += item; }
		return total;
	}`
	out := compile(t, ts)
	assertContains(t, out, "for _, item := range items")
}

func TestForInLoop(t *testing.T) {
	ts := `function keys(obj: any): void {
		for (const k in obj) { console.log(k); }
	}`
	out := compile(t, ts)
	assertContains(t, out, "for k := range obj")
}

func TestWhileLoop(t *testing.T) {
	ts := `function countdown(n: number): void {
		while (n > 0) { n--; }
	}`
	out := compile(t, ts)
	assertContains(t, out, "for n > 0")
}

func TestDoWhile(t *testing.T) {
	ts := `function f(): void {
		let i = 0;
		do { i++; } while (i < 10);
	}`
	out := compile(t, ts)
	assertContains(t, out, "for {")
	assertContains(t, out, "break")
}

func TestSwitchStatement(t *testing.T) {
	ts := `function describe(x: number): string {
		switch (x) {
			case 1: return "one";
			case 2: return "two";
			default: return "other";
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "switch x")
	assertContains(t, out, "case 1:")
	assertContains(t, out, "case 2:")
	assertContains(t, out, "default:")
}

func TestSwitchBreakStripped(t *testing.T) {
	ts := `function f(x: number): void {
		switch (x) {
			case 1: console.log("one"); break;
			case 2: console.log("two"); break;
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "case 1:")
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, "case 1:") || strings.Contains(line, "case 2:") {
			for j := i + 1; j < len(lines) && j < i+4; j++ {
				trimmed := strings.TrimSpace(lines[j])
				if trimmed == "break" {
					t.Errorf("found bare 'break' in switch case body at line %d", j)
				}
			}
		}
	}
}

func TestBreakAndContinue(t *testing.T) {
	ts := `function f(): void {
		for (let i = 0; i < 10; i++) {
			if (i === 5) break;
			if (i === 3) continue;
		}
	}`
	out := compile(t, ts)
	assertContains(t, out, "break")
	assertContains(t, out, "continue")
}

func TestThrowStatement(t *testing.T) {
	ts := `function fail(): void { throw new Error("boom"); }`
	out := compile(t, ts)
	assertContains(t, out, `panic(errors.New("boom"))`)
}

func TestTryCatch(t *testing.T) {
	ts := `function safe(): void {
		try { console.log("try"); } catch (e) { console.log("catch"); }
	}`
	out := compile(t, ts)
	assertContains(t, out, "defer func()")
	assertContains(t, out, "recover()")
}
