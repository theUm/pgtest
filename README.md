# pgtest
Tool to automate postgres integration tests.

### how it works 
1. creates empty `template` db
2. applies migrations from specified dir (via go-migrate)
3. on every `SetupChild(name)` function creates separate Postgres DB as a copy of `template` DB (with migrations included)
4. `SetupChild(name)` return config of just created DB, so you can connect to it in your test
5. after running a test connection to test db should be closed
6. `TeardownChild(name)` function deletes test db
7. `Teardown()` function deletes all undeleted by previos command DBs including all the test dbs and `template` DB

### db interactions diagram
![interactions diagram](.github/diagram.png?raw=true)
