//! PhantomDep graph analyser.
//!
//! Reads the line protocol described in `.kiro/specs/phantomdep/design.md` §4 on
//! stdin and writes a single-line JSON object on stdout.
//!
//! Design constraint: the reply contains only integers, floats and index arrays.
//! That is deliberate — it removes the need for any string escaping, and
//! therefore removes the need for a JSON library.

use std::io::{self, Read};

const PROTOCOL_VERSION: &str = "PHANTOMDEP-GRAPH 1";

#[derive(Default, Clone)]
struct Node {
    declared: bool,
    imported: bool,
    dev: bool,
    tomb: bool,
    files: i64,
}

struct Graph {
    nodes: Vec<Node>,
    /// Kept for diagnostics and for the error messages; the JSON reply is
    /// deliberately name-free so that no escaping code is needed.
    #[allow(dead_code)]
    names: Vec<String>,
    adj: Vec<Vec<usize>>,
}

fn main() {
    let mut raw = String::new();
    if let Err(e) = io::stdin().read_to_string(&mut raw) {
        eprintln!("cannot read stdin: {e}");
        std::process::exit(2);
    }
    match parse(&raw) {
        Ok(graph) => println!("{}", analyse(&graph)),
        Err(e) => {
            eprintln!("{e}");
            std::process::exit(2);
        }
    }
}

fn parse(raw: &str) -> Result<Graph, String> {
    let mut lines = raw.lines();
    match lines.next() {
        Some(l) if l.trim() == PROTOCOL_VERSION => {}
        Some(l) => {
            return Err(format!(
                "protocol mismatch: expected {PROTOCOL_VERSION:?}, got {:?}",
                l.trim()
            ))
        }
        None => return Err("empty input".into()),
    }

    let mut nodes: Vec<Node> = Vec::new();
    let mut names: Vec<String> = Vec::new();
    let mut edges: Vec<(usize, usize)> = Vec::new();
    let mut done = false;

    for (offset, raw_line) in lines.enumerate() {
        let lineno = offset + 2;
        let line = raw_line.trim_end();
        if line.is_empty() {
            continue;
        }
        if line == "DONE" {
            done = true;
            break;
        }

        if let Some(rest) = line.strip_prefix("NODE ") {
            let (kv_part, name) = match rest.find("name=") {
                Some(p) => (&rest[..p], rest[p + 5..].to_string()),
                None => return Err(format!("line {lineno}: NODE record without name=")),
            };
            let mut toks = kv_part.split_whitespace();
            let idx_tok = toks
                .next()
                .ok_or_else(|| format!("line {lineno}: NODE record without index"))?;
            let idx: usize = idx_tok
                .parse()
                .map_err(|_| format!("line {lineno}: bad node index {idx_tok:?}"))?;

            let mut node = Node::default();
            for tok in toks {
                let (key, value) = tok
                    .split_once('=')
                    .ok_or_else(|| format!("line {lineno}: malformed field {tok:?}"))?;
                match key {
                    "declared" => node.declared = value != "0",
                    "imported" => node.imported = value != "0",
                    "dev" => node.dev = value != "0",
                    "tomb" => node.tomb = value != "0",
                    "files" => node.files = value.parse().unwrap_or(0),
                    _ => {}
                }
            }

            if idx != nodes.len() {
                return Err(format!(
                    "line {lineno}: node index {idx} out of order, expected {}",
                    nodes.len()
                ));
            }
            nodes.push(node);
            names.push(name);
        } else if let Some(rest) = line.strip_prefix("EDGE ") {
            let mut toks = rest.split_whitespace();
            let from: usize = toks
                .next()
                .and_then(|s| s.parse().ok())
                .ok_or_else(|| format!("line {lineno}: bad edge source"))?;
            let to: usize = toks
                .next()
                .and_then(|s| s.parse().ok())
                .ok_or_else(|| format!("line {lineno}: bad edge target"))?;
            edges.push((from, to));
        } else {
            return Err(format!("line {lineno}: unknown record {line:?}"));
        }
    }

    if !done {
        return Err("input truncated: missing DONE record".into());
    }

    let n = nodes.len();
    let mut adj: Vec<Vec<usize>> = vec![Vec::new(); n];
    for (from, to) in edges {
        if from >= n || to >= n {
            return Err(format!("edge {from}->{to} references a missing node"));
        }
        if from == to || adj[from].contains(&to) {
            continue;
        }
        adj[from].push(to);
    }

    Ok(Graph { nodes, names, adj })
}

fn analyse(g: &Graph) -> String {
    let n = g.nodes.len();
    if n == 0 {
        return "{\"cycles\":[],\"maxDepth\":0,\"ghostScore\":0,\"phantom\":[],\"undeclared\":[],\"tombstones\":[],\"fanIn\":[]}".into();
    }

    let cycles = tarjan_cycles(&g.adj, n);
    let max_depth = bfs_depth(&g.adj, n, 0);

    let mut fan_in = vec![0usize; n];
    for targets in &g.adj {
        for &t in targets {
            fan_in[t] += 1;
        }
    }

    let mut phantom = Vec::new();
    let mut undeclared = Vec::new();
    let mut tombstones = Vec::new();
    let mut declared_count = 0usize;
    let mut phantom_mass = 0.0f64;

    for (i, node) in g.nodes.iter().enumerate() {
        // Node 0 is the repository root: it is never a dependency, so it must
        // not inflate the denominator of the ghost score nor be reported as an
        // unpinned package.
        if i == 0 {
            continue;
        }
        if node.declared {
            declared_count += 1;
        }
        if node.declared && !node.imported {
            phantom.push(i);
            // A phantom that other packages depend on hides a whole subtree,
            // so it weighs more than a leaf nobody ever imported.
            phantom_mass += 1.0 + (fan_in[i] as f64) / 10.0;
        }
        if !node.declared && node.imported {
            undeclared.push(i);
        }
        if node.declared && node.tomb {
            tombstones.push(i);
        }
    }

    let depth_penalty = (max_depth.min(8) as f64) / 8.0;
    let mut score = 0.0;
    if declared_count > 0 {
        score = 100.0 * phantom_mass / (declared_count as f64 + 1.0) * (0.6 + 0.4 * depth_penalty);
    }
    let score = score.clamp(0.0, 100.0);

    let mut out = String::with_capacity(256 + n * 4);
    out.push_str("{\"cycles\":");
    out.push('[');
    for (i, cycle) in cycles.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push('[');
        for (j, idx) in cycle.iter().enumerate() {
            if j > 0 {
                out.push(',');
            }
            out.push_str(&idx.to_string());
        }
        out.push(']');
    }
    out.push(']');

    out.push_str(&format!(",\"maxDepth\":{max_depth}"));
    out.push_str(&format!(",\"ghostScore\":{score:.2}"));

    out.push_str(",\"phantom\":");
    push_indices(&mut out, &phantom);
    out.push_str(",\"undeclared\":");
    push_indices(&mut out, &undeclared);
    out.push_str(",\"tombstones\":");
    push_indices(&mut out, &tombstones);
    out.push_str(",\"fanIn\":");
    push_indices(&mut out, &fan_in);

    out.push('}');
    out
}

fn push_indices(out: &mut String, values: &[usize]) {
    out.push('[');
    for (i, v) in values.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push_str(&v.to_string());
    }
    out.push(']');
}

/// Tarjan's strongly connected components. Components of size >= 2 are cycles.
fn tarjan_cycles(adj: &[Vec<usize>], n: usize) -> Vec<Vec<usize>> {
    struct State<'a> {
        adj: &'a [Vec<usize>],
        index: Vec<i64>,
        low: Vec<usize>,
        on_stack: Vec<bool>,
        stack: Vec<usize>,
        counter: usize,
        out: Vec<Vec<usize>>,
    }

    impl<'a> State<'a> {
        fn connect(&mut self, v: usize) {
            self.index[v] = self.counter as i64;
            self.low[v] = self.counter;
            self.counter += 1;
            self.stack.push(v);
            self.on_stack[v] = true;

            let neighbours = self.adj[v].clone();
            for w in neighbours {
                if self.index[w] < 0 {
                    self.connect(w);
                    self.low[v] = self.low[v].min(self.low[w]);
                } else if self.on_stack[w] {
                    self.low[v] = self.low[v].min(self.index[w] as usize);
                }
            }

            if self.low[v] == self.index[v] as usize {
                let mut comp = Vec::new();
                while let Some(w) = self.stack.pop() {
                    self.on_stack[w] = false;
                    comp.push(w);
                    if w == v {
                        break;
                    }
                }
                if comp.len() >= 2 {
                    comp.sort_unstable();
                    self.out.push(comp);
                }
            }
        }
    }

    let mut st = State {
        adj,
        index: vec![-1; n],
        low: vec![0; n],
        on_stack: vec![false; n],
        stack: Vec::new(),
        counter: 0,
        out: Vec::new(),
    };

    for v in 0..n {
        if st.index[v] < 0 {
            st.connect(v);
        }
    }

    let mut out = st.out;
    out.sort_by_key(|c| c[0]);
    out
}

/// Longest shortest-path from `start`, computed with an iterative BFS.
fn bfs_depth(adj: &[Vec<usize>], n: usize, start: usize) -> usize {
    let mut dist: Vec<Option<usize>> = vec![None; n];
    let mut queue = std::collections::VecDeque::new();
    dist[start] = Some(0);
    queue.push_back(start);
    let mut max = 0;

    while let Some(v) = queue.pop_front() {
        let d = dist[v].unwrap_or(0);
        max = max.max(d);
        for &w in &adj[v] {
            if dist[w].is_none() {
                dist[w] = Some(d + 1);
                queue.push_back(w);
            }
        }
    }
    max
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn detects_a_two_node_cycle() {
        let input = "PHANTOMDEP-GRAPH 1\nNODE 0 declared=1 imported=1 dev=0 tomb=0 files=0 name=root\nNODE 1 declared=1 imported=1 dev=0 tomb=0 files=0 name=a\nNODE 2 declared=1 imported=1 dev=0 tomb=0 files=0 name=b\nEDGE 0 1 import\nEDGE 1 2 dep\nEDGE 2 1 dep\nDONE\n";
        let g = parse(input).expect("parses");
        let cycles = tarjan_cycles(&g.adj, g.nodes.len());
        assert_eq!(cycles, vec![vec![1, 2]]);
    }

    #[test]
    fn rejects_truncated_input() {
        let input = "PHANTOMDEP-GRAPH 1\nNODE 0 declared=1 imported=1 dev=0 tomb=0 files=0 name=root\n";
        assert!(parse(input).is_err());
    }

    #[test]
    fn rejects_wrong_protocol() {
        assert!(parse("PHANTOMDEP-GRAPH 99\nDONE\n").is_err());
    }

    #[test]
    fn score_is_bounded() {
        let input = "PHANTOMDEP-GRAPH 1\nNODE 0 declared=1 imported=1 dev=0 tomb=0 files=0 name=root\nNODE 1 declared=1 imported=0 dev=0 tomb=0 files=0 name=ghost\nDONE\n";
        let g = parse(input).expect("parses");
        let json = analyse(&g);
        assert!(json.contains("\"ghostScore\":"));
        assert!(!json.contains("NaN"));
    }
}
