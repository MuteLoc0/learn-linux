package devcmds

import (
	"fmt"

	"learn-linux/internal/contract"
)

func tutorial(name string) contract.CommandHandler {
	return func(ec *contract.ExecContext, args []string) int {
		if ec.OnTutorialAction == nil {
			return 0
		}
		code, err := ec.OnTutorialAction(contract.TutorialAction{Name: name, Args: args})
		if err != nil {
			fmt.Fprintf(ec.Stderr, "%s: %v\n", name, err)
		}
		return code
	}
}
