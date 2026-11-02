package app

import (
  "bytes"
  "fmt"
  "html/template"
  github "jjoaovitor7/github-toplangs/internal/provider"
  "log"
  "net/http"
  "os"
  "path/filepath"
  "sort"
  "strconv"
  "strings"
  "sync"
  "time"
)

type SVGData struct {
  BgColor     string
  TitleColor  string
  BorderColor string
  Width       int
  Height      int
  Langs       []LangView
}

type LangView struct {
  Name     string
  NameY    int
  Percent  float64
  PercentX float64
  Width    float64
  BarY     int
  Color    string
  Bytes    int
  ColX     int
}

// https://raw.githubusercontent.com/github/linguist/master/lib/linguist/languages.yml
var langColors = map[string]string{
  "Go":         "#00ADD8",
  "JavaScript": "#f1e05a",
  "TypeScript": "#3178c6",
  "PHP":        "#4F5D95",
  "Lua":        "#000080",
  "Java":       "#b07219",
  "Kotlin":     "#A97BFF",
  "HTML":       "#e34c26",
  "CSS":        "#663399",
  "Svelte":     "#ff3e00",
  "MDX":        "#fcb32c",
  "Shell":      "#89e051",
  "C++":        "#f34b7d",
  "C#":         "#178600",
  "Python":     "#3572A5",
}

type svgCache struct {
  mu    sync.RWMutex
  items map[string][]byte
}

var cache = &svgCache{items: map[string][]byte{}}

func (c *svgCache) get(key string) ([]byte, bool) {
  c.mu.RLock()
  defer c.mu.RUnlock()

  svg, ok := c.items[key]
  return svg, ok
}

func (c *svgCache) set(key string, value []byte) {
  c.mu.Lock()
  defer c.mu.Unlock()

  c.items[key] = value
}

var (
  templatesDir     string
  topLangsTemplate *template.Template
  indexTemplate    *template.Template
)

func SetTemplatesDir() {
  templatesDir = os.Getenv("TEMPLATES_DIR")
  if templatesDir == "" {
    templatesDir = "templates"
  }

  topLangsTemplate = template.Must(template.ParseFiles(filepath.Join(templatesDir, "toplangs.tmpl")))
  indexTemplate = template.Must(template.ParseFiles(filepath.Join(templatesDir, "index.tmpl")))
}

func generateSVG(bgColor string, titleColor string, borderColor string, langs []LangView) ([]byte, error) {
  const (
    colWidth  = 128
    colLeftX  = 32
    colRightX = 128
    colStartY = 99
    colStepY  = 42
  )

  sum := 0
  for _, l := range langs {
    sum += l.Bytes
  }

  colLeft := (len(langs) + 1) / 2
  colRight := len(langs) - colLeft
  maxRows := colLeft

  if colRight > maxRows {
    maxRows = colRight
  }

  colX := []int{colLeftX, colRightX + 64}
  percentX := []int{colLeftX + colWidth, colRightX + colWidth + 64}

  for i := range langs {
    l := &langs[i]
    l.Percent = float64(l.Bytes) / float64(sum) * 100
    l.Width = (l.Percent / 100) * colWidth

    color := langColors[l.Name]
    if l.Color == "" {
      l.Color = "#ffffff"
    }
    l.Color = color

    col := 0
    row := i
    if i >= colLeft {
      col = 1
      row = i - colLeft
    }

    l.NameY = colStartY + row*colStepY
    l.BarY = l.NameY + 4
    l.ColX = colX[col]
    l.PercentX = float64(percentX[col])
  }

  data := SVGData{
    BgColor:     bgColor,
    TitleColor:  titleColor,
    BorderColor: borderColor,
    Width:       364,
    Height:      colStartY + colStepY*maxRows + 16,
    Langs:       langs,
  }

  var svgBuffer bytes.Buffer
  if err := topLangsTemplate.Execute(&svgBuffer, data); err != nil {
    return nil, err
  }

  return svgBuffer.Bytes(), nil
}

func topLangsHandler(username string, token string, limit int, hide map[string]bool) (map[string]int, error, int) {
  githubProvider := github.NewGitHubProvider(token)

  sum := make(map[string]int)
  repos, err, status := githubProvider.FetchRepos(username)

  if err != nil {
    return nil, err, status
  }

  for _, r := range repos {
    langs, err, status := githubProvider.FetchLangs(username, r.Name)
    if err != nil {
      return nil, err, status
    }

    for lang, bytes := range langs {
      lang = strings.ReplaceAll(lang, "#", "sharp")
      lang = strings.ReplaceAll(lang, "++", "pp")
      hideLang := strings.ToLower(lang)

      if !hide[hideLang] {
        sum[lang] += bytes
      }
    }
  }

  if limit > 0 && limit < len(sum) {
    keys := make([]string, 0, len(sum))
    for k := range sum {
      keys = append(keys, k)
    }

    sort.Slice(keys, func(i int, j int) bool {
      return sum[keys[i]] > sum[keys[j]]
    })

    topLangs := make(map[string]int, limit)
    for _, k := range keys[:limit] {
      topLangs[k] = sum[k]
    }
    sum = topLangs
  }
  return sum, nil, http.StatusOK
}

func isHexColor(value string) bool {
  if value == "" {
    return false
  }

  for _, r := range value {
    if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
      return false
    }
  }

  return true
}

func isColorName(value string) bool {
  if value == "" {
    return false
  }

  for _, r := range value {
    if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
      return false
    }
  }

  return true
}

func normalizeColor(value string) (string, bool) {
  value = strings.TrimSpace(value)
  if value == "" {
    return "", true
  }

  hex := strings.TrimPrefix(value, "#")
  switch len(hex) {
  case 3, 4, 6, 8:
    if isHexColor(hex) {
      return "#" + strings.ToLower(hex), true
    }
  }

  if isColorName(value) {
    return strings.ToLower(value), true
  }

  return "", false
}

func TopLangsRouteHandler(w http.ResponseWriter, r *http.Request) {
  token := os.Getenv("GITHUB_TOKEN")

  user := r.URL.Query().Get("user")
  if user == "" {
    http.Error(w, "Field 'user' is empty.", http.StatusBadRequest)
    return
  }

  bgColor, ok := normalizeColor(r.URL.Query().Get("bgcolor"))
  if !ok {
    http.Error(w, "Invalid 'bgcolor'. Use a hex value or a CSS color name.", http.StatusBadRequest)
    return
  }
  if bgColor == "" {
    bgColor = "#111"
  }

  titleColor, ok := normalizeColor(r.URL.Query().Get("titlecolor"))
  if !ok {
    http.Error(w, "Invalid 'titlecolor'. Use a hex value or a CSS color name.", http.StatusBadRequest)
    return
  }
  if titleColor == "" {
    titleColor = "#fff"
  }

  borderColor, ok := normalizeColor(r.URL.Query().Get("bordercolor"))
  if !ok {
    http.Error(w, "Invalid 'bordercolor'. Use a hex value or a CSS color name.", http.StatusBadRequest)
    return
  }
  if borderColor == "" {
    borderColor = "none"
  }

  limitQuery := r.URL.Query().Get("limit")
  var limit int = 8
  if n, err := strconv.Atoi(limitQuery); err == nil {
    limit = n
  }

  hideQuery := r.URL.Query().Get("hide")
  hide := map[string]bool{}
  if hideQuery != "" {
    for _, h := range strings.Split(hideQuery, ",") {
      hideLang := strings.TrimSpace(strings.ToLower(h))
      hide[hideLang] = true
    }
  }

  params := r.URL.Query()
  params.Del("t")
  cacheKey := params.Encode()

  if svg, ok := cache.get(cacheKey); ok {
    writeSVG(w, svg)
    return
  }

  data, err, status := topLangsHandler(user,
    token,
    limit,
    hide)
  if err != nil {
    log.Println(err)
    http.Error(w, err.Error(), status)
    return
  }

  var list []LangView
  for k, v := range data {
    list = append(list, LangView{
      Name:  k,
      Bytes: v,
    })
  }

  sort.Slice(list, func(i, j int) bool {
    return list[i].Bytes > list[j].Bytes
  })

  svg, err := generateSVG(bgColor,
    titleColor,
    borderColor,
    list)
  if err != nil {
    log.Println(err)
    http.Error(w, "Error generating SVG.", http.StatusInternalServerError)
    return
  }

  cache.set(cacheKey, svg)

  writeSVG(w, svg)
}

func writeSVG(w http.ResponseWriter, svg []byte) {
  w.Header().Set("Cache-Control", "public, max-age=43200, must-revalidate")
  w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
  w.Header().Set("Content-Length", strconv.Itoa(len(svg)))

  w.WriteHeader(http.StatusOK)
  w.Write(svg)
}

func IndexRouteHandler(w http.ResponseWriter, r *http.Request) {
  indexTemplate.Execute(w, nil)
  w.WriteHeader(http.StatusOK)
}

func HealthRouteHandler(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json; charset=utf-8")
  w.Header().Set("Cache-Control", "no-store")
  w.WriteHeader(http.StatusOK)
  fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().Format(time.RFC3339))
}
