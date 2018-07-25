package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type repositoriesResponse struct {
	Repositories []string `json:"repositories"`
}

func (registry *Registry) Repositories() ([]string, error) {
	repos := make([]string, 0, 10)

	rchan, echan := registry.StreamRepositories(context.Background())

	for {
		select {
		case r, ok := <-rchan:
			if !ok {
				return repos, nil
			}
			repos = append(repos, r)
		case e := <-echan:
			return repos, e
		}
	}
}

func (registry *Registry) StreamRepositories(ctx context.Context) (<-chan string, <-chan error) {
	regChan := make(chan string)
	errChan := make(chan error)

	go func() {
		// defer close(errChan)
		defer close(regChan)

		regurl := registry.url("/v2/_catalog")

		var err error //We create this here, otherwise url will be rescoped with :=
		var response repositoriesResponse

		for {
			select {
			case <-ctx.Done():
				return
			default:
				registry.Logf("registry.repositories url=%s", regurl)
				regurl, err = registry.getPaginatedJson(regurl, &response)
				switch err {
				case ErrNoMorePages:
					streamRegistryAPIRepositoriesPage(ctx, regChan, response.Repositories)
					return
				case nil:
					if !streamRegistryAPIRepositoriesPage(ctx, regChan, response.Repositories) {
						return
					}
					continue
				default:
					if ue, ok := err.(*url.Error); ok {
						if he, ok := ue.Err.(*HttpStatusError); ok {
							if he.Response.StatusCode == http.StatusUnauthorized {
								regurl = registry.url("/api/v0/repositories/")
								registry.Logf("attempting DTR fallback at %v", regurl)

								gotSome := false
								for {
									var err2 error

									select {
									case <-ctx.Done():
										return
									default:
										dtrRepositories := struct {
											Repositories []dtrRepository `json:"repositories"`
										}{}

										regurl, err2 = registry.getPaginatedJson(regurl, &dtrRepositories)

										switch err2 {
										case ErrNoMorePages:
											gotSome = true
											streamDTRAPIRepositoriesPage(ctx, regChan, dtrRepositories.Repositories)
											return
										case nil:
											gotSome = true
											if !streamDTRAPIRepositoriesPage(ctx, regChan, dtrRepositories.Repositories) {
												return
											}
											continue
										default:
											if gotSome {
												// we got something successfully but now we're failing, return the current error
												errChan <- err2
												return
											}

											// the DTR API didn't work, return the original error
											errChan <- err
											return
										}
									}
								}
							}
						}
					}

					errChan <- err
					return
				}
			}
		}
	}()

	return regChan, errChan
}

type dtrRepository struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

func streamRegistryAPIRepositoriesPage(ctx context.Context, c chan string, v []string) bool {
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

func streamDTRAPIRepositoriesPage(ctx context.Context, c chan string, v []dtrRepository) bool {
	for _, r := range v {
		select {
		case <-ctx.Done():
			return false
		case c <- fmt.Sprintf("%s/%s", r.Namespace, r.Name):
			// next
		}
	}
	return true
}
