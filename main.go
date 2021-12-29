package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/gomodule/redigo/redis"
	"os"
	"strconv"
	"strings"
)

func newPool(server *redisServer) *redis.Pool {
	return &redis.Pool{
		MaxIdle: 80,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {

			options := []redis.DialOption{}

			if server.Encrypted {
				options = []redis.DialOption{
					redis.DialTLSSkipVerify(true),
					redis.DialUseTLS(true),
				}
			}

			c, err := redis.Dial("tcp", fmt.Sprintf("%v:%v",server.Endpoint, server.Port), options...)

			if err != nil {
				panic(err.Error())
			}
			return c, err
		},
	}
}

func findRedisServers() ([]*redisServer, error){
	cfg, _ := config.LoadDefaultConfig(context.Background())
	ecClient := elasticache.NewFromConfig(cfg)
	
	servers, err := ecClient.DescribeCacheClusters(context.Background(), &elasticache.DescribeCacheClustersInput{})

	if err != nil {
		return nil, errors.New("user does not have permissions to describe cache clusters")
	}

	clusterMap := map[string]types.CacheCluster{}

	for _, cluster := range servers.CacheClusters {
		if strings.EqualFold("redis", aws.ToString(cluster.Engine)){
			_,_ = fmt.Fprintf(os.Stderr, "%v\n", aws.ToString(cluster.CacheClusterId))
			clusterMap[aws.ToString(cluster.CacheClusterId)] = cluster
		}

	}

	repGroups, err := ecClient.DescribeReplicationGroups(context.Background(), &elasticache.DescribeReplicationGroupsInput{})

	if err != nil {
		return nil, errors.New("user does not have permissions to describe replication groups")
	}


	redisServers := make([]*redisServer, 0)
	for _, replicationGroup := range repGroups.ReplicationGroups {

		if cluster, ok := clusterMap[replicationGroup.MemberClusters[0]]; ok {
			//_,_ = fmt.Fprintf(os.Stderr, "%v - %v\n", aws.ToString(replicationGroup.Name), aws.ToString(cluster.ConfigurationEndpoint.Address))

			_,_ = fmt.Fprintf(os.Stderr, "%v - %v\n", aws.ToString(replicationGroup.Description), aws.ToString(replicationGroup.NodeGroups[0].PrimaryEndpoint.Address))

			name := aws.ToString(replicationGroup.Description)

			tags, err := ecClient.ListTagsForResource(context.Background(), &elasticache.ListTagsForResourceInput{
				ResourceName: cluster.ARN,
			})

			if err != nil && tags != nil {
				for _, tag := range tags.TagList {
					if strings.EqualFold("name", aws.ToString(tag.Key)) {
						name = aws.ToString(tag.Value)
					}
				}
			}

			redisServers = append(redisServers, &redisServer{
				Endpoint:  aws.ToString(replicationGroup.NodeGroups[0].PrimaryEndpoint.Address),
				Port:      replicationGroup.NodeGroups[0].PrimaryEndpoint.Port,
				Name:      name,
				Encrypted: aws.ToBool(cluster.TransitEncryptionEnabled),
			})

		}

	}

	return redisServers, nil
}

var pool *redis.Pool

func main() {
	r := bufio.NewReader(os.Stdin)

	server := &redisServer{}


	if len(os.Args) > 1 {

		encrypted := !strings.EqualFold(os.Args[1], "localhost")

		server = &redisServer{
			Endpoint:  os.Args[1],
			Port:      6379,
			Name:      os.Args[1],
			Encrypted: encrypted,
		}
	}else {
		redisServers, err := findRedisServers()

		if err != nil {
			_,_ = fmt.Fprintf(os.Stderr, "%v\n", err)
			return
		}

		_,_ = fmt.Fprintf(os.Stderr, "Enter the server to connect to:\n")
		for i, server := range redisServers {
			_,_ = fmt.Fprintf(os.Stderr, "%v) %v %v\n", i + 1, server.Name, server.Endpoint)
		}

		s, _ := r.ReadString('\n')
		selection, _ := strconv.Atoi(strings.TrimSpace(s))

		if ( selection -1) >= len(redisServers)  {
			return
		}

		server = redisServers[selection -1]

	}

	_,_ = fmt.Fprintf(os.Stderr, "Connecting to %v...\n", server.Name)

	pool = newPool(server)
	client := pool.Get()
	defer client.Close()

	for {


		args := strings.Split(prompt(r), " ")

		command := args[0]

		commandArgs := make([]interface{},0 )
		for x:= 1; x< len(args); x++{
			commandArgs = append(commandArgs, args[x])
		}

		returnValue, err := client.Do(command, commandArgs...)
		if err != nil {
			_,_ = fmt.Fprintf(os.Stderr, "%v\n", err)
			continue
		}

		switch val := returnValue.(type){
		case string:
			_,_ = fmt.Fprintf(os.Stderr, "%v\n", returnValue)
		case []uint8:
			_,_ = fmt.Fprintf(os.Stderr, "%v\n", string(val))
		case []interface{}:
			for _, inner := range val {
				switch val := inner.(type){
				case string:
					_,_ = fmt.Fprintf(os.Stderr, "%v\n", inner)
				case []uint8:
					_,_ = fmt.Fprintf(os.Stderr, "%v\n", string(val))
				}
			}
		}

	}

}

func prompt(r *bufio.Reader) string{
	_,_ = fmt.Fprintf(os.Stderr, "redis> ")

	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}
//
//func main2() {
//
//	var url string
//	var useTls bool
//	flag.StringVar(&url, "host", "", "")
//	flag.BoolVar(&useTls, "tls", true, "")
//	flag.Parse()
//
//	args := flag.Args()
//
//
//}