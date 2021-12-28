package main

import (
	"flag"
	"fmt"
	"github.com/gomodule/redigo/redis"
)

func newPool(url string, useTls bool) *redis.Pool {
	return &redis.Pool{
		MaxIdle: 80,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {

			options := []redis.DialOption{}

			if useTls {
				options = []redis.DialOption{redis.DialTLSSkipVerify(true)}
			}

			c, err := redis.Dial("tcp", url, options...)

			if err != nil {
				panic(err.Error())
			}
			return c, err
		},
	}
}



var pool *redis.Pool

func main() {

	var url string
	var useTls bool
	flag.StringVar(&url, "host", "", "")
	flag.BoolVar(&useTls, "tls", true, "")
	flag.Parse()

	args := flag.Args()

	pool = newPool(url, useTls)

	command := args[0]

	commandArgs := make([]interface{},0 )
	for x:= 1; x< len(args); x++{
		commandArgs = append(commandArgs, args[x])
	}

	client := pool.Get()
	defer client.Close()

	returnValue, err := client.Do(command, commandArgs...)
	if err != nil {
		panic(err)
	}

	switch val := returnValue.(type){
	case string:
		fmt.Println(returnValue)
	case []uint8:
		fmt.Println(string(val))
	case []interface{}:
		for _, inner := range val {
			switch val := inner.(type){
			case string:
				fmt.Println(inner)
			case []uint8:
				fmt.Println(string(val))
			}
		}
	}

}