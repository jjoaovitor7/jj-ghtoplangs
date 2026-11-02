package provider

import (
  "encoding/json"
  "errors"
  "fmt"
  "log"
  "net/http"
)

type Repo struct {
  Name      string `json:"name"`
  CreatedAt string `json:"created_at"`
}

type GitHubProvider struct {
  GITHUB_API string
  Token      string
  Client     *http.Client
}

func NewGitHubProvider(token string) GitHubProvider {
  return GitHubProvider{
    GITHUB_API: "https://api.github.com",
    Token:      token,
    Client:     &http.Client{},
  }
}

func apiError(statusCode int, notFoundMessage string) (error, int) {
  switch statusCode {
  case http.StatusNotFound:
    return errors.New(notFoundMessage), http.StatusNotFound
  case http.StatusUnauthorized:
    return errors.New("GitHub token is missing or invalid."), http.StatusBadGateway
  case http.StatusForbidden:
    return errors.New("GitHub API rate limit exceeded. Try again later."), http.StatusBadGateway
  default:
    return errors.New("GitHub error."), http.StatusBadGateway
  }
}

func (g GitHubProvider) FetchRepos(username string) ([]Repo, error, int) {
  url := fmt.Sprintf("%s/users/%s/repos?per_page=99", g.GITHUB_API, username)

  req, _ := http.NewRequest("GET", url, nil)
  req.Header.Set("Authorization", fmt.Sprintf("token %s", g.Token))

  res, err := g.Client.Do(req)
  if err != nil {
    log.Println(err)
    return nil, errors.New("Error getting repositories."), http.StatusInternalServerError
  }
  defer res.Body.Close()

  if res.StatusCode != http.StatusOK {
    err, status := apiError(res.StatusCode, "User not found.")
    return nil, err, status
  }

  var repos []Repo
  if err := json.NewDecoder(res.Body).Decode(&repos); err != nil {
    log.Println(err)
    return nil, errors.New("Server error."), http.StatusInternalServerError
  }

  return repos, nil, http.StatusOK
}

func (g GitHubProvider) FetchLangs(username, repo string) (map[string]int, error, int) {
  url := fmt.Sprintf("%s/repos/%s/%s/languages", g.GITHUB_API, username, repo)

  req, _ := http.NewRequest("GET", url, nil)
  req.Header.Set("Authorization", fmt.Sprintf("token %s", g.Token))

  res, err := g.Client.Do(req)
  if err != nil {
    log.Println(err)
    return nil, errors.New("Error getting langs."), http.StatusInternalServerError
  }
  defer res.Body.Close()

  if res.StatusCode != http.StatusOK {
    err, status := apiError(res.StatusCode, "Repository not found.")
    return nil, err, status
  }

  var langs map[string]int
  if err := json.NewDecoder(res.Body).Decode(&langs); err != nil {
    log.Println(err)
    return nil, errors.New("Server error."), http.StatusInternalServerError
  }

  return langs, nil, http.StatusOK
}
