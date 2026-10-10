'use strict';
process.env.R269_CHECKS='1';process.env.R266_CASE_ONE='1';process.env.R266_BACKEND=process.env.R269_BACKEND || process.env.R266_BACKEND || '/tmp/r269-public';process.env.R266_BROWSER_OUT=process.env.R269_BROWSER_OUT || process.env.R266_BROWSER_OUT || '/tmp/r269-browser';require('./r266-browser.cjs');
