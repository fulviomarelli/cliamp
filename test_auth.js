const puppeteer = require('puppeteer');

(async () => {
  const browser = await puppeteer.launch({ headless: false });
  const page = await browser.newPage();
  await page.goto('https://music.apple.com');
  // wait and print
  console.log('wait for login...');
  let tokens = await page.evaluate(() => {
     return {
         dev: window.MusicKit.getInstance().developerToken,
         user: window.MusicKit.getInstance().musicUserToken,
     };
  });
  console.log(tokens);
  // wait 30 seconds
  await new Promise(r => setTimeout(r, 30000));
  await browser.close();
})();
