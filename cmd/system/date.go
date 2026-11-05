package system

import (
	"cli/internal/cli"
	"cli/internal/meta"
	"fmt"
	"time"
)

func NewDateCmd() *cli.Command {
	n := meta.Name

	return &cli.Command{
		Name:        "date",
		Aliases:     []string{"time"},
		Description: "Show the current date and time",
		Long:        "The optional format uses Go's reference time: 2006-01-02 15:04:05 MST.",
		Args:        "[format]",
		Examples: []string{
			fmt.Sprintf("%s system date             # 2006-01-02 15:04:05 MSTZ07:00", n),
			fmt.Sprintf("%s system date 2006-01-02", n),
			fmt.Sprintf("%s system time 15:04:05", n),
		},
		Run: func(args []string) error {
			format := "2006-01-02 15:04:05 MSTZ07:00"
			if len(args) > 0 {
				format = args[0]
			}
			fmt.Println(time.Now().Format(format))
			return nil
		},
	}
}
