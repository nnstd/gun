package compiler

import "testing"

func TestYargsCommandBuilderAndHandler(t *testing.T) {
	ts := `
import yargs from 'yargs';
import { hideBin } from 'yargs/helpers';

yargs.default(hideBin(process.argv))
  .command('greet <name>', 'greet someone', (yargs) => {
    return yargs.positional('name', { type: 'string' })
  }, (argv) => {
    console.log(argv.name)
  })
  .parse()
`
	out := compile(t, ts)
	// Builder callback should have *yargs.Yargs parameter and return type
	assertContains(t, out, "func(yargs *yargs.Yargs) *yargs.Yargs")
	// Handler callback should have *yargs.Argv parameter
	assertContains(t, out, "func(argv *yargs.Argv)")
	// Builder body should call methods on the yargs parameter, not the package
	assertContains(t, out, "yargs.Positional(")
	// Handler body should use Get() for property access
	assertContains(t, out, `argv.Get("name")`)
}
