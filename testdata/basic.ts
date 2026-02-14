// Variables
const greeting: string = "hello";
let count: number = 42;
const items: number[] = [1, 2, 3];

// Function
function add(a: number, b: number): number {
  return a + b;
}

// Arrow function
const multiply = (x: number, y: number): number => x * y;

// Interface (data)
interface User {
  name: string;
  age: number;
  email: string;
}

// Interface (methods)
interface Reader {
  read(buf: string): number;
  close(): void;
}

// Class
class Animal {
  name: string;
  sound: string;

  constructor(name: string, sound: string) {
    this.name = name;
    this.sound = sound;
  }

  speak(): string {
    return `${this.name} says ${this.sound}`;
  }
}

// Enum
enum Color {
  Red,
  Green,
  Blue,
}

// String enum
enum Direction {
  Up = "UP",
  Down = "DOWN",
  Left = "LEFT",
  Right = "RIGHT",
}

// Control flow
function processItems(items: number[]): number {
  let sum: number = 0;

  for (const item of items) {
    if (item > 10) {
      continue;
    }
    sum += item;
  }

  while (sum < 100) {
    sum *= 2;
  }

  switch (sum) {
    case 0:
      console.log("zero");
      break;
    case 1:
      console.log("one");
      break;
    default:
      console.log("other");
  }

  return sum;
}

// Type alias
type StringMap = Record<string, string>;

// Error handling
function safeParse(input: string): any {
  try {
    const result = JSON.parse(input);
    return result;
  } catch (e) {
    console.error(e);
    return null;
  }
}

// String methods
function formatName(name: string): string {
  const trimmed = name.trim();
  const upper = name.toUpperCase();
  const hasAt = name.startsWith("@");
  return `${trimmed} (${upper}) hasAt=${hasAt}`;
}

// Export
export function greet(name: string): string {
  return `Hello, ${name}!`;
}
