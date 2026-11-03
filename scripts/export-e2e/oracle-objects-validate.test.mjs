import assert from 'node:assert/strict'
import test from 'node:test'
import { createTargets } from './oracle-objects-validate.mjs'

test('忽略注释、字符串和普通名称引用中的伪 CREATE', () => {
  const sql = `-- CREATE PACKAGE "missing";
/* CREATE TYPE "missing"; */
SELECT 'CREATE TRIGGER missing';
CREATE TABLE "synthetic" ("CREATE" VARCHAR2(30) DEFAULT 'CREATE VIEW missing');`
  assert.deepEqual(createTargets(sql), [{ objectType: 'TABLE', name: 'synthetic' }])
})

test('Oracle 的固定分隔符包装仍须核对实际目标和类型', () => {
  for (const type of ['FUNCTION', 'PROCEDURE', 'PACKAGE', 'TRIGGER', 'TYPE', 'VIEW']) {
    assert.deepEqual(createTargets(`DELIMITER $$\nCREATE OR REPLACE ${type} "synthetic_schema"."synthetic_object" AS BEGIN NULL; END;\n$$\nDELIMITER ;`), [{ objectType: type, name: 'synthetic_object' }])
  }
})

test('私有和公共同义词分别核对目标，包体不冒充包定义', () => {
  assert.deepEqual(createTargets('CREATE SYNONYM "same" FOR "source"."target"; CREATE PUBLIC SYNONYM "same" FOR "target"; CREATE PACKAGE BODY "pkg" AS END;'), [
    { objectType: 'SYNONYM', name: 'same', forName: 'target' },
    { objectType: 'PUBLIC_SYNONYM', name: 'same', forName: 'target' },
    { objectType: 'PACKAGE_BODY', name: 'pkg' },
  ])
  assert.throws(() => createTargets('CREATE PUBLIC SYNONYM "bad" AS SELECT 1;'), /SYNONYM_TARGET_MISSING/)
})
