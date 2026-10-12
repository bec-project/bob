import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import { analyzeCommits } from '@semantic-release/commit-analyzer';
import { generateNotes } from '@semantic-release/release-notes-generator';

const config = JSON.parse(await readFile(new URL('../.releaserc.json', import.meta.url)));
const pluginOptions = (name) => config.plugins.find(([plugin]) => plugin === name)[1];

for (const [message, release, section, text] of [
  ['fix: preserve home ownership (#42)', 'patch', 'Bug Fixes', 'preserve home ownership'],
  ['feat: add a provider (#42)', 'minor', 'Features', 'add a provider'],
  ['feat!: change config (#42)\n\nBREAKING CHANGE: migrate config', 'major', 'BREAKING CHANGES', 'migrate config'],
]) {
  test(`${release} releases generate notes with the configured preset`, async () => {
    const context = {
      cwd: process.cwd(),
      logger: { log() {} },
      options: { repositoryUrl: 'https://github.com/bec-project/bob' },
      commits: [{ message, hash: '1234567890abcdef' }],
      lastRelease: { version: '1.0.0', gitTag: 'v1.0.0' },
      nextRelease: { version: '2.0.0', gitTag: 'v2.0.0' },
    };
    assert.equal(await analyzeCommits(pluginOptions('@semantic-release/commit-analyzer'), context), release);
    const notes = await generateNotes(pluginOptions('@semantic-release/release-notes-generator'), context);
    assert.ok(notes.includes(section), notes);
    assert.ok(notes.includes(text), notes);
    assert.ok(notes.includes('https://github.com/bec-project/bob/issues/42'), notes);
    assert.ok(notes.includes('https://github.com/bec-project/bob/compare/v1.0.0...v2.0.0'), notes);
  });
}
