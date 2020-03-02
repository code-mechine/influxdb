package kv_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	influxdb "github.com/influxdata/influxdb"
	"github.com/influxdata/influxdb/inmem"
	"github.com/influxdata/influxdb/kv"
	"go.uber.org/zap"
)

func Test_Migrator(t *testing.T) {
	var (
		ctx            = context.TODO()
		store          = inmem.NewKVStore()
		logger         = zap.NewNop()
		migrationOne   = newMigration("migration one")
		migrationTwo   = newMigration("migration two")
		migrationThree = newMigration("migration three")
		migrationFour  = newMigration("migration four")
		migrator       = kv.NewMigrator(logger,
			// all migrations excluding number four (for now)
			migrationOne,
			migrationTwo,
			migrationThree,
		)

		// mocking now time
		timestamp = int64(0)
		now       = func() time.Time {
			timestamp++
			return time.Unix(timestamp, 0).In(time.UTC)
		}

		// ts returns a point to a time at N unix seconds.
		ts = func(n int64) *time.Time {
			t := time.Unix(n, 0).In(time.UTC)
			return &t
		}
	)

	kv.MigratorSetNow(t, migrator, now)

	if err := migrator.Initialize(ctx, store); err != nil {
		t.Fatal(err)
	}

	t.Run("List() shows all migrations in down state", func(t *testing.T) {
		migrations, err := migrator.List(ctx, store)
		if err != nil {
			t.Fatal(err)
		}

		if expected := []kv.Migration{
			{
				ID:    influxdb.ID(1),
				Name:  "migration one",
				State: kv.DownMigrationState,
			},
			{
				ID:    influxdb.ID(2),
				Name:  "migration two",
				State: kv.DownMigrationState,
			},
			{
				ID:    influxdb.ID(3),
				Name:  "migration three",
				State: kv.DownMigrationState,
			},
		}; !reflect.DeepEqual(expected, migrations) {
			t.Errorf("expected %#v, found %#v", expected, migrations)
		}
	})

	t.Run("Up() runs each migration in turn", func(t *testing.T) {
		// apply all migrations
		if err := migrator.Up(ctx, store); err != nil {
			t.Fatal(err)
		}

		// list migration again
		migrations, err := migrator.List(ctx, store)
		if err != nil {
			t.Fatal(err)
		}

		if expected := []kv.Migration{
			{
				ID:         influxdb.ID(1),
				Name:       "migration one",
				State:      kv.UpMigrationState,
				StartedAt:  ts(1),
				FinishedAt: ts(2),
			},
			{
				ID:         influxdb.ID(2),
				Name:       "migration two",
				State:      kv.UpMigrationState,
				StartedAt:  ts(3),
				FinishedAt: ts(4),
			},
			{
				ID:         influxdb.ID(3),
				Name:       "migration three",
				State:      kv.UpMigrationState,
				StartedAt:  ts(5),
				FinishedAt: ts(6),
			},
		}; !reflect.DeepEqual(expected, migrations) {
			t.Errorf("expected %#v, found %#v", expected, migrations)
		}

		// assert each migration was called
		migrationOne.assertUpCalled(t, 1)
		migrationTwo.assertUpCalled(t, 1)
		migrationThree.assertUpCalled(t, 1)
	})

	t.Run("List() after adding new migration it reports as expected", func(t *testing.T) {
		migrator.AddMigrations(migrationFour)

		// list migration again
		migrations, err := migrator.List(ctx, store)
		if err != nil {
			t.Fatal(err)
		}

		if expected := []kv.Migration{
			{
				ID:         influxdb.ID(1),
				Name:       "migration one",
				State:      kv.UpMigrationState,
				StartedAt:  ts(1),
				FinishedAt: ts(2),
			},
			{
				ID:         influxdb.ID(2),
				Name:       "migration two",
				State:      kv.UpMigrationState,
				StartedAt:  ts(3),
				FinishedAt: ts(4),
			},
			{
				ID:         influxdb.ID(3),
				Name:       "migration three",
				State:      kv.UpMigrationState,
				StartedAt:  ts(5),
				FinishedAt: ts(6),
			},
			{
				ID:    influxdb.ID(4),
				Name:  "migration four",
				State: kv.DownMigrationState,
			},
		}; !reflect.DeepEqual(expected, migrations) {
			t.Errorf("expected %#v, found %#v", expected, migrations)
		}
	})

	t.Run("Up() only applies the single down migration", func(t *testing.T) {
		// apply all migrations
		if err := migrator.Up(ctx, store); err != nil {
			t.Fatal(err)
		}

		// list migration again
		migrations, err := migrator.List(ctx, store)
		if err != nil {
			t.Fatal(err)
		}

		if expected := []kv.Migration{
			{
				ID:         influxdb.ID(1),
				Name:       "migration one",
				State:      kv.UpMigrationState,
				StartedAt:  ts(1),
				FinishedAt: ts(2),
			},
			{
				ID:         influxdb.ID(2),
				Name:       "migration two",
				State:      kv.UpMigrationState,
				StartedAt:  ts(3),
				FinishedAt: ts(4),
			},
			{
				ID:         influxdb.ID(3),
				Name:       "migration three",
				State:      kv.UpMigrationState,
				StartedAt:  ts(5),
				FinishedAt: ts(6),
			},
			{
				ID:         influxdb.ID(4),
				Name:       "migration four",
				State:      kv.UpMigrationState,
				StartedAt:  ts(7),
				FinishedAt: ts(8),
			},
		}; !reflect.DeepEqual(expected, migrations) {
			t.Errorf("expected %#v, found %#v", expected, migrations)
		}

		// assert each migration was called only once
		migrationOne.assertUpCalled(t, 1)
		migrationTwo.assertUpCalled(t, 1)
		migrationThree.assertUpCalled(t, 1)
		migrationFour.assertUpCalled(t, 1)
	})

	t.Run("Down() calls down for each migration", func(t *testing.T) {
		// apply all migrations
		if err := migrator.Down(ctx, store); err != nil {
			t.Fatal(err)
		}

		// list migration again
		migrations, err := migrator.List(ctx, store)
		if err != nil {
			t.Fatal(err)
		}

		if expected := []kv.Migration{
			{
				ID:    influxdb.ID(1),
				Name:  "migration one",
				State: kv.DownMigrationState,
			},
			{
				ID:    influxdb.ID(2),
				Name:  "migration two",
				State: kv.DownMigrationState,
			},
			{
				ID:    influxdb.ID(3),
				Name:  "migration three",
				State: kv.DownMigrationState,
			},
			{
				ID:    influxdb.ID(4),
				Name:  "migration four",
				State: kv.DownMigrationState,
			},
		}; !reflect.DeepEqual(expected, migrations) {
			t.Errorf("expected %#v, found %#v", expected, migrations)
		}

		// assert each migration was called only once
		migrationOne.assertDownCalled(t, 1)
		migrationTwo.assertDownCalled(t, 1)
		migrationThree.assertDownCalled(t, 1)
		migrationFour.assertDownCalled(t, 1)
	})

	t.Run("Up() re-applies all migrations", func(t *testing.T) {
		// apply all migrations
		if err := migrator.Up(ctx, store); err != nil {
			t.Fatal(err)
		}

		// list migration again
		migrations, err := migrator.List(ctx, store)
		if err != nil {
			t.Fatal(err)
		}

		if expected := []kv.Migration{
			{
				ID:         influxdb.ID(1),
				Name:       "migration one",
				State:      kv.UpMigrationState,
				StartedAt:  ts(9),
				FinishedAt: ts(10),
			},
			{
				ID:         influxdb.ID(2),
				Name:       "migration two",
				State:      kv.UpMigrationState,
				StartedAt:  ts(11),
				FinishedAt: ts(12),
			},
			{
				ID:         influxdb.ID(3),
				Name:       "migration three",
				State:      kv.UpMigrationState,
				StartedAt:  ts(13),
				FinishedAt: ts(14),
			},
			{
				ID:         influxdb.ID(4),
				Name:       "migration four",
				State:      kv.UpMigrationState,
				StartedAt:  ts(15),
				FinishedAt: ts(16),
			},
		}; !reflect.DeepEqual(expected, migrations) {
			t.Errorf("expected %#v, found %#v", expected, migrations)
		}

		// assert each migration up was called for a second time
		migrationOne.assertUpCalled(t, 2)
		migrationTwo.assertUpCalled(t, 2)
		migrationThree.assertUpCalled(t, 2)
		migrationFour.assertUpCalled(t, 2)
	})

	t.Run("List() missing migration spec errors as expected", func(t *testing.T) {
		// remove last specification from migration list
		migrator.Migrations = migrator.Migrations[:len(migrator.Migrations)-1]
		// list migration again
		_, err := migrator.List(ctx, store)
		if !errors.Is(err, kv.ErrMigrationSpecNotFound) {
			t.Errorf("expected migration spec error, found %v", err)
		}
	})
}

func newMigration(name string) *spyMigrationSpec {
	return &spyMigrationSpec{name: name}
}

type spyMigrationSpec struct {
	name       string
	upCalled   int
	downCalled int
}

func (s *spyMigrationSpec) Name() string {
	return s.name
}

func (s *spyMigrationSpec) assertUpCalled(t *testing.T, times int) {
	t.Helper()
	if s.upCalled != times {
		t.Errorf("expected Up() to be called %d times, instead found %d times", times, s.upCalled)
	}
}

func (s *spyMigrationSpec) Up(ctx context.Context, store kv.Store) error {
	s.upCalled++
	return nil
}

func (s *spyMigrationSpec) assertDownCalled(t *testing.T, times int) {
	t.Helper()
	if s.downCalled != times {
		t.Errorf("expected Down() to be called %d times, instead found %d times", times, s.downCalled)
	}
}

func (s *spyMigrationSpec) Down(ctx context.Context, store kv.Store) error {
	s.downCalled++
	return nil
}
