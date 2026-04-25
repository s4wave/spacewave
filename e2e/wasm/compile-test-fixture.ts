// Trivial test fixture for validating the esbuild compilation pipeline.
// Exports a default function that returns a greeting string.
export default function (args: {
  name: string
}): { greeting: string } {
  return { greeting: 'hello ' + args.name }
}
