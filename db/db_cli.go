package db

import (
	"os"

	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	// _ imports the boltdb
	_ "github.com/cayleygraph/cayley/graph/kv/bolt"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// Kind is a cli database kind string.
type Kind string

const (
	// INMEM is an in-memory database.
	INMEM Kind = "inmem"
	// BOLT uses bolt-db.
	BOLT Kind = "bolt"
)

// CLIOpts are options to construct the database from the cli.
type CLIOpts struct {
	// Kind is the kind of database to use.
	// [inmem, bolt]
	Kind string
	// Path is the path to the database.
	Path string
}

// BuildDatabase builds the database.
func (o *CLIOpts) BuildDatabase(le *logrus.Entry) (*Database, error) {
	// Create a brand new graph
	var store *graph.Handle
	var err error
	le.WithField("kind", o.Kind).Debug("building database")
	switch o.Kind {
	case string(INMEM):
		store, err = cayley.NewMemoryGraph()
	default:
		if err := os.MkdirAll(o.Path, 0755); err != nil {
			le.WithError(err).Warn("unable to mkdir db path")
			return nil, err
		}

		_ = graph.InitQuadStore(string(o.Kind), o.Path, nil)
		store, err = cayley.NewGraph(string(o.Kind), o.Path, nil)
	}
	if err != nil {
		le.WithError(err).Error("error building db")
		return nil, err
	}

	le.Debug("database built")
	return NewDatabase(store, le)
}

// BuildCLIFlags builds the cli flags.
func (o *CLIOpts) BuildCLIFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:        "db",
			Usage:       "db type to use, from [inmem, bolt]",
			Destination: &o.Kind,
			Value:       "inmem",
		},
		cli.StringFlag{
			Name:        "db-path",
			Usage:       "path to the db file to use",
			Destination: &o.Path,
			Value:       "hydra.db",
		},
	}
}

/*
func main() {
	app := cli.NewApp()
	app.Name = "dbtest"
	opts := &CLIOpts{}
	app.Flags = opts.BuildCLIFlags()

	app.Action = func(c *cli.Context) error {
		log := logrus.New()
		log.SetLevel(logrus.DebugLevel)
		le := logrus.NewEntry(log)

		db, err := opts.BuildDatabase(le)
		if err != nil {
			return err
		}
		defer db.Close()

		store := db.Handle
		store.AddQuad(quad.Make("phrase of the day", "is of course", "Hello World!", nil))

		// Now we create the path, to get to our data
		p := cayley.StartPath(store, quad.String("phrase of the day")).Out(quad.String("is of course"))

		// Now we iterate over results. Arguments:
		// 1. Optional context used for cancellation.
		// 2. Flag to optimize query before execution.
		// 3. Quad store, but we can omit it because we have already built path with it.
		return p.Iterate(nil).EachValue(nil, func(value quad.Value) {
			nativeValue := quad.NativeOf(value) // this converts RDF values to normal Go types
			fmt.Println(nativeValue)
		})
	}

	if err := app.Run(os.Args); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
	}
}
*/
