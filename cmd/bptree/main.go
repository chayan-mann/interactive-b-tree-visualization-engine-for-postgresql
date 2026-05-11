// Command bptree is a small REPL for the in-memory B+ tree engine.
//
// Supported commands (case-insensitive):
//
//	INSERT <key> [value]
//	DELETE <key>
//	SEARCH <key>
//	RANGE <lo> <hi>
//	PRINT
//	SNAPSHOT
//	HELP
//	QUIT
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/indexlab/indexlab/internal/bptree"
)

func main() {
	order := flag.Int("order", 4, "B+ tree order (max children per internal node)")
	flag.Parse()

	tree := bptree.New(bptree.Config{Order: *order})
	fmt.Printf("B+ tree REPL (order=%d). Type HELP for commands.\n", *order)

	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !sc.Scan() {
			return
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := strings.ToUpper(parts[0])
		switch cmd {
		case "HELP":
			printHelp()
		case "QUIT", "EXIT":
			return
		case "INSERT":
			if len(parts) < 2 {
				fmt.Println("usage: INSERT <key> [value]")
				continue
			}
			k, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("invalid key")
				continue
			}
			value := ""
			if len(parts) >= 3 {
				value = strings.Join(parts[2:], " ")
			} else {
				value = fmt.Sprintf("row-%d", k)
			}
			tree.Insert(k, value)
			fmt.Printf("inserted %d -> %q\n", k, value)
		case "DELETE":
			if len(parts) != 2 {
				fmt.Println("usage: DELETE <key>")
				continue
			}
			k, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("invalid key")
				continue
			}
			if tree.Delete(k) {
				fmt.Printf("deleted %d\n", k)
			} else {
				fmt.Printf("key %d not present\n", k)
			}
		case "SEARCH":
			if len(parts) != 2 {
				fmt.Println("usage: SEARCH <key>")
				continue
			}
			k, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("invalid key")
				continue
			}
			if v, ok := tree.Search(k); ok {
				fmt.Printf("found %d -> %q\n", k, v)
			} else {
				fmt.Printf("key %d not found\n", k)
			}
		case "RANGE":
			if len(parts) != 3 {
				fmt.Println("usage: RANGE <lo> <hi>")
				continue
			}
			lo, err1 := strconv.Atoi(parts[1])
			hi, err2 := strconv.Atoi(parts[2])
			if err1 != nil || err2 != nil {
				fmt.Println("invalid range")
				continue
			}
			results := tree.RangeSearch(lo, hi)
			for _, kv := range results {
				fmt.Printf("  %d -> %q\n", kv.Key, kv.Value)
			}
			fmt.Printf("(%d rows)\n", len(results))
		case "PRINT":
			fmt.Println(tree.String())
			fmt.Print("leaf chain: ")
			fmt.Println(tree.KeysInOrder())
		case "SNAPSHOT":
			snap := tree.Snapshot()
			b, _ := json.MarshalIndent(snap, "", "  ")
			fmt.Println(string(b))
		default:
			fmt.Printf("unknown command: %s (try HELP)\n", cmd)
		}
	}
}

func printHelp() {
	fmt.Println(`Commands:
  INSERT <key> [value]   insert or update a key
  DELETE <key>           remove a key
  SEARCH <key>           look up a key
  RANGE <lo> <hi>        range scan, inclusive
  PRINT                  print the tree by levels and leaf chain
  SNAPSHOT               print snapshot as JSON
  QUIT                   exit`)
}
