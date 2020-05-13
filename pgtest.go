package pgtest

import (
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" //required by go-migrate
	_ "github.com/golang-migrate/migrate/v4/source/file"       //required by go-migrate
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"regexp"
)

const (
	active childStatus = iota
	shutdown
)

type childStatus int

type PgConfig struct {
	Host     string
	Port     int
	Db       string
	User     string
	Password string
	Ssl      bool
}

type Config struct {
	MigrationsPath string
	TemplateDBName string
	MasterConfig   *PgConfig
}

type ChildDB struct {
	config *PgConfig
	status childStatus
}

type childConfigs map[string]*ChildDB

type PgTest struct {
	config           *Config
	masterConnection *sqlx.DB
	ChildrenConfigs  childConfigs
}

func New(config *Config) (*PgTest, error) {
	masterConn, err := sqlx.Connect("postgres", config.MasterConfig.ConnectionString())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to master DB")
	}

	return &PgTest{
		config:           config,
		masterConnection: masterConn,
		ChildrenConfigs:  make(childConfigs),
	}, nil
}

func (p *PgTest) Setup() error {
	if err := p.createTemplateDB(); err != nil {
		return errors.Wrap(err, "create template db")
	}
	if err := p.migrateOnTemplate(); err != nil {
		return errors.Wrap(err, "apply migrations")
	}
	return nil
}

func (p *PgTest) TearDown() error {
	_, err := p.masterConnection.Exec(fmt.Sprintf("drop database if exists %s;", p.config.TemplateDBName))
	if err != nil {
		return errors.Wrapf(err, "failed to drop database %s", p.config.TemplateDBName)
	}

	for _, childDB := range p.ChildrenConfigs {
		if childDB.status == active {
			_, err := p.masterConnection.Exec(fmt.Sprintf("drop database if exists %s;", childDB.config.Db))
			if err != nil {
				return errors.Wrapf(err, "failed to drop database %s", childDB.config.Db)
			}
		}
	}
	return nil
}

// (re)creates empty template db
func (p PgTest) createTemplateDB() error {
	//https://github.com/golang-migrate/migrate/issues/226
	_, err := p.masterConnection.Exec(fmt.Sprintf("drop database if exists %s;", p.config.TemplateDBName))
	if err != nil {
		return errors.Wrapf(err, "failed to drop database %s", p.config.TemplateDBName)
	}

	_, err = p.masterConnection.Exec(fmt.Sprintf("create database %s", p.config.TemplateDBName))
	if err != nil {
		return errors.Wrapf(err, "failed to create database %s", p.config.TemplateDBName)
	}

	return nil
}

//connects to template DB and applies migrations to it
func (p PgTest) migrateOnTemplate() error {
	//copy the master config & change db name
	dbConfig := *p.config.MasterConfig
	dbConfig.Db = p.config.TemplateDBName
	gm, err := migrate.New(p.config.MigrationsPath, dbConfig.ConnectionString())
	if err != nil {
		return errors.Wrap(err, "failed to connect to DB from go-MigrateTemplate")
	}

	//apply all migrations until its done
	//nolint fmt
	for {
		err = gm.Up()
		if err == migrate.ErrNoChange {
			break
		}
		if err != nil {
			return errors.Wrap(err, "migrate up")
		}
	}
	if _, err = gm.Close(); err != nil {
		return errors.Wrap(err, "failed to close DB")
	}
	return nil
}

// can be called as per-test basis. name
func (p PgTest) SetupChild(name string) (*PgConfig, error) {
	if name == "" && regexp.MustCompile(`^[A-Za-z0-9\-_]+$`).MatchString(name) {
		return nil, errors.New("invalid test db name. try something like \"test_user-unique\"")
	}

	cConfig := *p.config.MasterConfig
	cConfig.Db = name
	cConfigPtr := &cConfig
	err := p.ChildrenConfigs.add(name, cConfigPtr)
	if err != nil {
		return nil, errors.Wrap(err, "add child config")
	}

	//https://github.com/lib/pq/issues/694
	tbName := pq.QuoteIdentifier(name)
	_, err = p.masterConnection.Exec(fmt.Sprintf("drop database if exists %s;", tbName))
	if err != nil {
		return nil, errors.Wrap(err, "drop database for test")
	}

	_, err = p.masterConnection.Exec(fmt.Sprintf("create database %s template %s;", tbName, p.config.TemplateDBName))
	if err != nil {
		return nil, errors.Wrap(err, "create database for test")
	}

	return cConfigPtr, nil
}

func (p PgTest) TeardownChild(name string) error {
	cfg, err := p.ChildrenConfigs.get(name)
	if err != nil {
		return err
	}
	tbName := pq.QuoteIdentifier(name)
	_, err = p.masterConnection.Exec(fmt.Sprintf("drop database %s;", tbName))
	if err != nil {
		return errors.Wrapf(err, "drop test database %s", tbName)
	}
	cfg.status = shutdown
	return nil
}

func (cc childConfigs) add(name string, config *PgConfig) error {
	if _, ok := cc[name]; ok {
		return errors.New("database config with such name is already present")
	}
	cc[name] = &ChildDB{
		config: config,
		status: active,
	}
	return nil
}

func (cc childConfigs) get(name string) (*ChildDB, error) {
	if _, ok := cc[name]; !ok {
		return nil, errors.New("child database config not found")
	}
	return cc[name], nil
}

func (config PgConfig) ConnectionString() string {
	address := fmt.Sprintf("postgres://%v:%v@%v:%v/%v?sslmode=", config.User, config.Password, config.Host, config.Port, config.Db)
	if !config.Ssl {
		address += "disable"
	}

	return address
}
