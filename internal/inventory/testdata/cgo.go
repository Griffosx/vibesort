package fixtures

/*
int answer(void) { return 42; }
*/
import "C"

func UsesC() int { return int(C.answer()) }
