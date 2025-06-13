package main

func fibonacci(n int, a int, b int) int {
    if n == 0 {
        return a
    }
    return fibonacci(n-1, b, a+b)
}

func main() {
    println(fibonacci(10, 0, 1))
}
