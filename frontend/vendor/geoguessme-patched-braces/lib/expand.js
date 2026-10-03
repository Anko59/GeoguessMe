'use strict';

const fill = require('fill-range');
const stringify = require('./stringify');
const utils = require('./utils');
const { MAX_DEPTH } = require('./constants');

const append = (queue = '', stash = '', enclose = false) => {
  const result = [];

  queue = [].concat(queue);
  stash = [].concat(stash);

  if (!stash.length) return queue;
  if (!queue.length) {
    return enclose ? utils.flatten(stash).map(ele => `{${ele}}`) : stash;
  }

  for (const item of queue) {
    if (Array.isArray(item)) {
      for (const value of item) {
        result.push(append(value, stash, enclose));
      }
    } else {
      for (let ele of stash) {
        if (enclose === true && typeof ele === 'string') ele = `{${ele}}`;
        result.push(Array.isArray(ele) ? append(item, ele, enclose) : item + ele);
      }
    }
  }
  return utils.flatten(result);
};

const expand = (ast, options = {}) => {
  const rangeLimit = options.rangeLimit === undefined ? 1000 : options.rangeLimit;

  const requestedMaxDepth = options.maxDepth;
  const maxDepth = Number.isFinite(requestedMaxDepth) ? Math.min(MAX_DEPTH, requestedMaxDepth) : MAX_DEPTH;

  const stringifyNode = (node, depth) => stringify(node, Object.create(options, {
    maxDepth: { value: maxDepth - depth + (node.type === 'root' ? 0 : 1) }
  }));
  const enclosingBlock = node => {
    let depth = 0;
    while (node.type !== 'brace' && node.type !== 'root' && node.parent) {
      if (++depth > maxDepth) {
        throw new RangeError(`AST parent depth (${depth}), exceeds max depth (${maxDepth})`);
      }
      node = node.parent;
    }
    return node;
  };

  const walk = (node, parent = {}, depth = 0) => {
    utils.validateValues(node);
    if (node.nodes && depth > maxDepth) {
      throw new RangeError(`AST depth (${depth}), exceeds max depth (${maxDepth})`);
    }
    node.queue = [];

    const p = enclosingBlock(parent);
    const q = p.queue;

    if (node.invalid || node.dollar) {
      q.push(append(q.pop(), stringifyNode(node, depth)));
      return;
    }

    if (node.type === 'brace' && node.invalid !== true && node.nodes.length === 2) {
      q.push(append(q.pop(), ['{}']));
      return;
    }

    if (node.nodes && node.ranges > 0) {
      const args = utils.reduce(node.nodes);

      if (utils.exceedsLimit(...args, options.step, rangeLimit)) {
        throw new RangeError('expanded array length exceeds range limit. Use options.rangeLimit to increase or disable the limit.');
      }

      let range = fill(...args, options);
      if (range.length === 0) {
        range = stringifyNode(node, depth);
      }

      q.push(append(q.pop(), range));
      node.nodes = [];
      return;
    }

    const enclose = utils.encloseBrace(node);
    const block = enclosingBlock(node);
    const queue = block.queue;

    for (let i = 0; i < node.nodes.length; i++) {
      const child = node.nodes[i];

      if (child.type === 'comma' && node.type === 'brace') {
        if (i === 1) queue.push('');
        queue.push('');
        continue;
      }

      if (child.type === 'close') {
        q.push(append(q.pop(), queue, enclose));
        continue;
      }

      if (child.value && child.type !== 'open') {
        queue.push(append(queue.pop(), child.value));
        continue;
      }

      if (child.nodes) {
        walk(child, node, child.nodes ? depth + 1 : depth);
      }
    }

    return queue;
  };

  return utils.flatten(walk(ast, {}, ast.type === 'root' ? 0 : 1));
};

module.exports = expand;
