package main

import (
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/how"
)

func cmdHow(args []string) {
	out, err := how.Render(currentProject().Path, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}
