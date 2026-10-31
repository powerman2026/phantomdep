// Fixture source: `chalk` is a declared devDependency, so this import makes it
// live. `ghost-missing-dep` is declared nowhere, which is the undeclared case.
const chalk = require('chalk');
const ghost = require('ghost-missing-dep');

function haunt(message) {
  return chalk.gray(ghost.echo(message));
}

module.exports = { haunt };
