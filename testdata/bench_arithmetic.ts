function runBench(n: number): number {
  let acc = 0
  let x = 1
  let y = 2

  for (let i = 0; i < n; i++) {
    acc = ((acc + x) * 3 - y) / 2
    x = x + 1
    y = y + 2
  }

  return acc
}

console.log(runBench(2_000_000))
