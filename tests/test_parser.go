package main

import (
	"fmt"

	"github.com/Fred78290/cakeagent/pkg"
)

func main() {
	// Test cases for the mount string parser
	testCases := []string{
		"share:/mnt/share",
		"data:/data,uid=1000,gid=1000",
		"logs:/var/log,uid=0,gid=0,ro",
		"config:/etc/config,ro,early",
		"workspace:/workspace,uid=1001,gid=1001,readonly,early",
		"invalid-format",
		"share:/mnt/share,uid=invalid",
		"share:/mnt/share,unknown=option",
	}

	for i, testCase := range testCases {
		fmt.Printf("\n=== Test %d: %s ===\n", i+1, testCase)

		mount, err := pkg.ParseMountString(testCase)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
		} else {
			fmt.Printf("SUCCESS:\n")
			fmt.Printf("  Name: %s\n", mount.Name)
			fmt.Printf("  Target: %s\n", mount.Target)
			fmt.Printf("  UID: %d\n", mount.Uid)
			fmt.Printf("  GID: %d\n", mount.Gid)
			fmt.Printf("  Readonly: %t\n", mount.Readonly)
			fmt.Printf("  Early: %t\n", mount.Early)
		}
	}
}
