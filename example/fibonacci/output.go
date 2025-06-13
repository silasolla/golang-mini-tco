package main

func fibonacci(n int, a int, b int) int {
	for n != 0 {
		tmpn := n - 1
		tmpa := b
		tmpb := a + b
		n, a, b = tmpn, tmpa, tmpb
	}
	return a

}

func main() {
	println(fibonacci(10, 0, 1))
}
