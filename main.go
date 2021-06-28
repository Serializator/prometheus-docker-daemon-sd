package main

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var cli *client.Client
var config *Config
var sds []*SD

func init() {
	var err error
	cli, err = client.NewClientWithOpts()
	if err != nil {
		panic(err)
	}

	config, err = NewConfig("config.yml")
	if err != nil {
		panic(err)
	}

	sds = make([]*SD, len(config.Output))
	for index, output := range config.Output {
		sds[index] = &SD{
			File:     output.File,
			Criteria: output.Criteria,
			SDEntry:  make(map[string]SDEntry),
		}
	}
}

func main() {
	finished := make(chan bool)

	for _, sd := range sds {
		containers, err := getContainersByCriteria(sd.Criteria)
		if err != nil {
			panic(err)
		}

		for _, container := range containers {
			sd.AddOrUpdateEntry(container)
		}

		sd.NewWriter().Write()
	}

	go listen()
	<-finished
}

func getContainersByCriteria(criteria MatchCriteria) ([]types.Container, error) {
	filter := filters.NewArgs()
	criteria.ApplyToFilter(filter)

	return cli.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: filter,
	})

}

func listen() {
	filter := filters.NewArgs()

	// TODO: also update the output if a network change occurs

	filter.Add("type", "container")
	filter.Add("event", "start")
	filter.Add("event", "die")

	msgChan, errChan := cli.Events(context.Background(), types.EventsOptions{
		Filters: filter,
	})

	for {
		select {
		case err := <-errChan:
			panic(err)
		case msg := <-msgChan:
			if msg.Status == "start" {
				container, err := getContainerById(msg.ID)
				if err != nil {
					panic(err)
				}

				for _, sd := range sds {
					if !sd.Criteria.Match(container) {
						continue
					}

					sd.AddOrUpdateEntry(*container)
					sd.NewWriter().Write()
				}
			}

			if msg.Status == "die" {
				container, err := getContainerById(msg.ID)
				if err != nil {
					panic(err)
				}

				for _, sd := range sds {
					if !sd.Criteria.Match(container) {
						continue
					}

					sd.RemoveEntry(*container)
					sd.NewWriter().Write()
				}
			}
		}
	}
}

func getContainerById(id string) (*types.Container, error) {
	filter := filters.NewArgs()
	filter.Add("id", id)

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: filter,
	})

	if err != nil {
		return nil, err
	}

	if len(containers) < 1 {
		return nil, fmt.Errorf("unable to find container by ID \"%s\"", id)
	}

	return &containers[0], nil
}
