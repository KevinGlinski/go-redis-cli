package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/gomodule/redigo/redis"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func newPool(server *redisServer) *redis.Pool {
	return &redis.Pool{
		MaxIdle:   80,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {

			options := []redis.DialOption{}

			if server.Encrypted {
				options = []redis.DialOption{
					redis.DialTLSSkipVerify(true),
					redis.DialUseTLS(true),
				}
			}

			c, err := redis.Dial("tcp", fmt.Sprintf("%v:%v", server.Endpoint, server.Port), options...)

			if err != nil {
				panic(err.Error())
			}
			return c, err
		},
	}
}

func findRedisServers(region string) ([]*redisServer, error) {
	cfg, _ := config.LoadDefaultConfig(context.Background())
	cfg.Region = region
	ecClient := elasticache.NewFromConfig(cfg)

	servers, err := ecClient.DescribeCacheClusters(context.Background(), &elasticache.DescribeCacheClustersInput{})

	if err != nil {
		print("%v", err)
		return nil, errors.New("user does not have permissions to describe cache clusters")
	}

	clusterMap := map[string]types.CacheCluster{}

	for _, cluster := range servers.CacheClusters {
		if strings.EqualFold("redis", aws.ToString(cluster.Engine)) {
			print("%v\n", aws.ToString(cluster.CacheClusterId))
			clusterMap[aws.ToString(cluster.CacheClusterId)] = cluster
		}

	}

	repGroups, err := ecClient.DescribeReplicationGroups(context.Background(), &elasticache.DescribeReplicationGroupsInput{})

	if err != nil {
		print("%v", err)
		return nil, errors.New("user does not have permissions to describe replication groups")
	}

	redisServers := make([]*redisServer, 0)
	for _, replicationGroup := range repGroups.ReplicationGroups {

		if cluster, ok := clusterMap[replicationGroup.MemberClusters[0]]; ok {
			//_,_ = fmt.Fprintf(os.Stderr, "%v - %v\n", aws.ToString(replicationGroup.Name), aws.ToString(cluster.ConfigurationEndpoint.Address))

			print("%v - %v\n", aws.ToString(replicationGroup.Description), aws.ToString(replicationGroup.NodeGroups[0].PrimaryEndpoint.Address))

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

func print(format string, params ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, format, params)
}

func discoverRegionFromMetadata() string {
	url := "http://169.254.169.254/latest/api/token"
	method := "PUT"

	client := &http.Client {
	}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		fmt.Println(err)
		return ""
	}
	req.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	token := string(body)

	req, err = http.NewRequest("GET",  "http://169.254.169.254/latest/meta-data//placement/availability-zone", nil)

	if err != nil {
		fmt.Println(err)
		return ""
	}
	req.Header.Add("X-aws-ec2-metadata-token", token)

	res, err = client.Do(req)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer res.Body.Close()

	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	re := regexp.MustCompile(`(?m)[a-z]$`)

	return re.ReplaceAllString(string(body), "")

}

func main() {
	var region string
	flag.StringVar(&region, "region", "", "")
	flag.Parse()

	if len(region) == 0 {
		region = discoverRegionFromMetadata()
	}

	args := flag.Args()

	r := bufio.NewReader(os.Stdin)

	server := &redisServer{}

	if len(args) > 1 {

		encrypted := !strings.EqualFold(args[0], "localhost")

		server = &redisServer{
			Endpoint:  args[0],
			Port:      6379,
			Name:      args[0],
			Encrypted: encrypted,
		}
	} else {
		redisServers, err := findRedisServers(region)

		if err != nil {
			print("%v\n", err)
			return
		}

		print("Enter the server to connect to:\n")
		for i, server := range redisServers {
			print("%v) %v %v\n", i+1, server.Name, server.Endpoint)
		}

		s, _ := r.ReadString('\n')
		selection, _ := strconv.Atoi(strings.TrimSpace(s))

		if (selection - 1) >= len(redisServers) {
			return
		}

		server = redisServers[selection-1]

	}

	_, _ = fmt.Fprintf(os.Stderr, "Connecting to %v...\n", server.Name)

	pool = newPool(server)
	client := pool.Get()
	defer client.Close()

	for {
		args := strings.Split(prompt(r), " ")

		command := args[0]

		commandArgs := make([]interface{}, 0)
		for x := 1; x < len(args); x++ {
			commandArgs = append(commandArgs, args[x])
		}

		returnValue, err := client.Do(command, commandArgs...)
		if err != nil {
			print("%v\n", err)
			continue
		}

		switch val := returnValue.(type) {
		case string:
			print("%v\n", returnValue)
		case []uint8:
			print("%v\n", string(val))
		case []interface{}:
			for _, inner := range val {
				switch val := inner.(type) {
				case string:
					print("%v\n", inner)
				case []uint8:
					print("%v\n", string(val))
				}
			}
		}

	}

}

func prompt(r *bufio.Reader) string {
	print("redis> ")

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
