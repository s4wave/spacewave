
const path = require('path');
const fs = require('fs');

const scriptsDir = path.join(process.cwd(), 'scripts');
const runtimeDir = path.join(scriptsDir, '../runtime')
const goQueryDir = path.join(runtimeDir, 'query')
const schemaFile = path.join(goQueryDir, 'schema.graphql')
const schemaGoFile = path.join(goQueryDir, 'schema.go')
const schemaTsFile = path.join(scriptsDir, '../src/schema/schema.ts')

let schemaStr = fs.readFileSync(schemaFile, 'utf8')
fs.writeFileSync(schemaGoFile, `package query

const SchemaSrc = \`${schemaStr}\`
`)
fs.writeFileSync(schemaTsFile, `import { buildSchema } from 'graphql'

// schema is the system schema
export const schema = buildSchema(\`
${schemaStr}
\`)
`)
// process.chdir(path.join(scriptsDir, 'build-resolvers'));
console.log(process.cwd());
require('./build-resolvers');