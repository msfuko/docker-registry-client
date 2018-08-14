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

											// try Harbor fallback
											regurl = registry.url("/api/projects")
											registry.Logf("attempting Harbor fallback at %v", regurl)
											gotSome := false
											for {
												var err3 error
												select {
												case <-ctx.Done():
													return
												default:
													harborProjects := []harborProject{}

													regurl, err3 = registry.getPaginatedJson(regurl, &harborProjects)

													switch err3 {
													case ErrNoMorePages:
														gotSome = true
														streamHarborProjectsPage(ctx, registry, regChan, errChan, harborProjects)
														return
													case nil:
														gotSome = true
														if !streamHarborProjectsPage(ctx, registry, regChan, errChan, harborProjects) {
															return
														}
														continue
													default:
														if gotSome {
															// we got something successfully but now we're failing, return the current error
															errChan <- err3
															return
														}

														// the fallbacks didn't work, return the original error
														errChan <- err
														return
													}
												}
											}
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

type harborProject struct {
	ID        int `json:"project_id"`
	RepoCount int `json:"repo_count"`
	// there are more fields but we don't care about them
}

type harborRepo struct {
	Name string `json:"name"`
	// there are more fields but we don't care about them
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

func streamHarborProjectsPage(ctx context.Context, registry *Registry, c chan string, e chan error, v []harborProject) bool {
	for _, project := range v {
		if project.RepoCount <= 0 {
			continue
		}

		if !streamHarborProjectRepos(ctx, project, registry, c, e) {
			return false
		}
	}

	return true
}

func streamHarborProjectRepos(ctx context.Context, project harborProject, registry *Registry, c chan string, e chan error) bool {
	u := registry.url(fmt.Sprintf("/api/repositories?project_id=%d", project.ID))

	for {
		var err error

		select {
		case <-ctx.Done():
			return false
		default:
			harborRepos := []harborRepo{}

			u, err = registry.getPaginatedJson(u, &harborRepos)

			switch err {
			case ErrNoMorePages:
				streamHarborReposPage(ctx, c, harborRepos)
				return true
			case nil:
				if !streamHarborReposPage(ctx, c, harborRepos) {
					return false
				}
				continue
			default:
				// we got something successfully but now we're failing, return the current error
				e <- err
				return false
			}
		}
	}
}

func streamHarborReposPage(ctx context.Context, c chan string, v []harborRepo) bool {
	for _, r := range v {
		select {
		case <-ctx.Done():
			return false
		case c <- r.Name:
			// next
		}
	}
	return true
}
