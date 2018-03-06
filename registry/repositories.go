package registry

import (
	"context"
)

type repositoriesResponse struct {
	Repositories []string `json:"repositories"`
}

func (registry *Registry) Repositories() ([]string, error) {
	url := registry.url("/v2/_catalog")
	repos := make([]string, 0, 10)
	var err error //We create this here, otherwise url will be rescoped with :=
	var response repositoriesResponse
	for {
		registry.Logf("registry.repositories url=%s", url)
		url, err = registry.getPaginatedJson(url, &response)
		switch err {
		case ErrNoMorePages:
			repos = append(repos, response.Repositories...)
			return repos, nil
		case nil:
			repos = append(repos, response.Repositories...)
			continue
		default:
			return nil, err
		}
	}
}

func (registry *Registry) StreamRepositories(ctx context.Context) (<-chan string, <-chan error) {
	regChan := make(chan string)
	errChan := make(chan error)

	go func() {
		// defer close(errChan)
		defer close(regChan)

		url := registry.url("/v2/_catalog")

		var err error //We create this here, otherwise url will be rescoped with :=
		var response repositoriesResponse

		for {
			select {
			case <-ctx.Done():
				return
			default:
				registry.Logf("registry.repositories url=%s", url)
				url, err = registry.getPaginatedJson(url, &response)
				switch err {
				case ErrNoMorePages:
					if !registry.streamPage(ctx, regChan, response.Repositories) {
						return
					}
					return
				case nil:
					if !registry.streamPage(ctx, regChan, response.Repositories) {
						return
					}
					continue
				default:
					errChan <- err
					return
				}
			}
		}
	}()

	return regChan, errChan
}

func (registry *Registry) streamPage(ctx context.Context, c chan string, v []string) bool {
	for _, r := range v {
		select {
		case <-ctx.Done():
			return false
		case c <- r:
			// next
		}
	}
	return true
}
