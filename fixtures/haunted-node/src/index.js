// Fixture source: imports three of the eight declared packages, so the other
// five are reported as phantom dependencies.
import express from 'express';
import lodash from 'lodash';

const app = express();

app.get('/', (req, res) => {
  res.json({ ok: true, chunked: lodash.chunk([1, 2, 3, 4], 2) });
});

export default app;
