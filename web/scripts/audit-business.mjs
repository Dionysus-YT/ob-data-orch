import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { analyzeSource, isTest, readSources, valueCycles } from './architecture/sources.mjs'
import { dependencyReview, dependencyViolation, networkViolations, ownershipReviews, responsibilityReviews } from './architecture/rules.mjs'

export function auditBusinessSources(records) {
  const violations = []
  const reviews = []
  const sources = new Map()
  const graph = new Map()
  for (const [file, text] of records) {
    if (isTest(file)) continue
    const source = analyzeSource(file, text, records)
    sources.set(file, source)
    graph.set(file, source.dependencies.filter(dep => !dep.typeOnly && dep.resolved && !isTest(dep.resolved)).map(dep => dep.resolved))
    for (const dep of source.dependencies) {
      const violation = dependencyViolation(file, dep)
      if (violation) violations.push(`${file}: ${violation}`)
      const review = dependencyReview(file, dep)
      if (review) reviews.push(`${file}: ${review}`)
    }
    violations.push(...networkViolations(source).map(item => `${file}: ${item}`))
    reviews.push(...responsibilityReviews(source).map(item => `${file}: ${item}`))
  }
  violations.push(...valueCycles(graph).map(chain => `循环值依赖: ${chain.join(' → ')}`))
  reviews.push(...ownershipReviews(sources, graph))
  return { violations: [...new Set(violations)], reviews: [...new Set(reviews)] }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const rootIndex = process.argv.indexOf('--root')
  const root = rootIndex === -1 ? resolve(dirname(fileURLToPath(import.meta.url)), '..') : resolve(process.argv[rootIndex + 1])
  const result = auditBusinessSources(readSources(root))
  result.reviews.forEach(item => console.info(`REVIEW ${item}`))
  result.violations.forEach(item => console.error(`FAIL ${item}`))
  console.info(`Business architecture: ${result.violations.length} violations, ${result.reviews.length} review signals`)
  if (result.violations.length) process.exitCode = 1
}
