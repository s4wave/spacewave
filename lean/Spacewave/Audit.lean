import Lean

/-!
# Axiom audit

`#assert_standard_axioms` fails elaboration when any declaration from a module
under `Spacewave` depends on an axiom other than Lean's standard three. Together
with `warningAsError`, which rejects `sorry`, it keeps every proof complete and
every assumption an explicit hypothesis.
-/

open Lean Elab Command

namespace Spacewave

/-- standardAxioms are the axioms core Lean's own theorems use. -/
def standardAxioms : List Name := [``propext, ``Classical.choice, ``Quot.sound]

/-- `#assert_standard_axioms` audits every imported `Spacewave` declaration. -/
elab "#assert_standard_axioms" : command => do
  let env ← getEnv
  let mut checked := 0
  for (name, _) in env.constants.map₁.toList do
    let some idx := env.getModuleIdxFor? name | continue
    unless (`Spacewave).isPrefixOf env.header.moduleNames[idx.toNat]! do continue
    checked := checked + 1
    let extra := (← liftCoreM (collectAxioms name)).filter (!standardAxioms.contains ·)
    unless extra.isEmpty do
      throwError "{name} depends on nonstandard axioms {extra}"
  if checked == 0 then
    throwError "no Spacewave declarations found to audit"

end Spacewave
