package main

type redisServer struct {
	Endpoint  string
	Port      int32
	Name      string
	Encrypted bool
}
