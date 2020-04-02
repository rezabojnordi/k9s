package dao

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/derailed/k9s/internal"
	"github.com/derailed/k9s/internal/client"
	cfg "github.com/derailed/k9s/internal/config"
	"github.com/derailed/k9s/internal/render"
	"github.com/derailed/popeye/pkg"
	"github.com/derailed/popeye/pkg/config"
	"github.com/derailed/popeye/types"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ Accessor = (*Popeye)(nil)

// Popeye tracks cluster sanitization.
type Popeye struct {
	NonResource
}

// NewPopeye returns a new set of aliases.
func NewPopeye(f Factory) *Popeye {
	a := Popeye{}
	a.Init(f, client.NewGVR("popeye"))

	return &a
}

type readWriteCloser struct {
	*bytes.Buffer
}

// Close close read stream.
func (readWriteCloser) Close() error {
	return nil
}

// List returns a collection of aliases.
func (p *Popeye) List(ctx context.Context, _ string) ([]runtime.Object, error) {
	defer func(t time.Time) {
		log.Debug().Msgf("Popeye -- Elapsed %v", time.Since(t))
	}(time.Now())

	flags := config.NewFlags()
	js := "json"
	flags.Output = &js

	if report, ok := ctx.Value(internal.KeyPath).(string); ok && report != "" {
		sections := []string{report}
		flags.Sections = &sections
	}
	spinach := filepath.Join(cfg.K9sHome, "spinach.yml")
	if c, err := p.Factory.Client().Config().CurrentContextName(); err == nil {
		spinach = filepath.Join(cfg.K9sHome, fmt.Sprintf("%s_spinach.yml", c))
	}
	if _, err := os.Stat(spinach); err == nil {
		flags.Spinach = &spinach
	}

	popeye, err := pkg.NewPopeye(flags, &log.Logger)
	if err != nil {
		return nil, err
	}
	popeye.SetFactory(newPopFactory(p.Factory))
	if err = popeye.Init(); err != nil {
		return nil, err
	}

	buff := readWriteCloser{Buffer: bytes.NewBufferString("")}
	popeye.SetOutputTarget(buff)
	if err = popeye.Sanitize(); err != nil {
		log.Debug().Msgf("BOOM %#v", *flags.Sections)
		return nil, err
	}

	var b render.Builder
	if err = json.Unmarshal(buff.Bytes(), &b); err != nil {
		return nil, err
	}

	oo := make([]runtime.Object, 0, len(b.Report.Sections))
	sort.Sort(b.Report.Sections)
	for _, s := range b.Report.Sections {
		s.Tally.Count = len(s.Outcome)
		if s.Tally.Sum() > 0 {
			oo = append(oo, s)
		}
	}

	return oo, nil
}

// Get fetch a resource.
func (a *Popeye) Get(_ context.Context, _ string) (runtime.Object, error) {
	return nil, errors.New("NYI!!")
}

// Helpers...

type popFactory struct {
	Factory
}

var _ types.Factory = (*popFactory)(nil)

func newPopFactory(f Factory) *popFactory {
	return &popFactory{Factory: f}
}
func (p *popFactory) Client() types.Connection {
	return &popConnection{Connection: p.Factory.Client()}
}

type popConnection struct {
	client.Connection
}

var _ types.Connection = (*popConnection)(nil)

func (c *popConnection) Config() types.Config {
	return c.Connection.Config()
}