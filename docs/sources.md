# Sources

The [README](../README.md) covers what freehire is; this is the full per-source
breakdown behind the "3.3M+ jobs" headline. Snapshot of 2026-10-20 —
**3,371,563 unique open postings** from **304** sources that currently carry one
(**320** adapters are registered in all). A posting is counted once: reposts of
the same job across boards are collapsed (a posting two sources carry is still
counted under each, so the sections sum slightly higher), and these are the figures
[`/jobs?source=<key>`](https://freehire.me/jobs) shows. The live, always-current
version of this page is [freehire.me/sources](https://freehire.me/sources). Every
source is one of three kinds:

- **ATS platforms** — one adapter per multi-tenant applicant-tracking system,
  each serving many companies (Workday, Greenhouse, Lever, iCIMS…).
- **Aggregators & job boards** — third-party feeds that republish many
  companies' postings (Adzuna, trudvsem, himalayas, jobtech, Telegram…).
- **Company career sites** — direct single-company feeds crawled from a
  company's own careers page (Amazon, Apple, Google, Yandex, Sber…).

A source's kind is not configuration: it falls out of the adapter's own Go type
in `internal/ingest/sources`, so the split below is derived, not maintained by hand.

## ATS platforms

**117 platforms with postings · 2,936,676 open postings.**

| Source | Open jobs |
| --- | ---: |
| workday | 596,262 |
| trudvsem | 347,656 |
| oracle | 287,895 |
| smartrecruiters | 154,777 |
| successfactors | 119,466 |
| greenhouse | 119,207 |
| ukg | 117,364 |
| paycom | 79,589 |
| icims | 67,182 |
| manatal | 63,385 |
| paylocity | 58,780 |
| dayforce | 56,170 |
| workable | 53,822 |
| ashby | 49,439 |
| bamboohr | 45,577 |
| taleo | 42,934 |
| teamtailor | 42,371 |
| adp | 41,159 |
| jazzhr | 39,866 |
| lever | 37,855 |
| hireology | 34,581 |
| paycor | 32,620 |
| zohorecruit | 30,658 |
| ukgready | 29,705 |
| adpmyjobs | 29,634 |
| phenom | 24,225 |
| breezy | 23,873 |
| recruitee | 22,210 |
| hrmdirect | 19,775 |
| applicantpro | 19,396 |
| rippling | 16,204 |
| isolvedhire | 15,665 |
| personio | 13,946 |
| eightfold | 12,986 |
| gupy | 12,216 |
| pinpoint | 11,717 |
| radancy | 11,346 |
| careerplug | 10,942 |
| join | 10,533 |
| hiringthing | 9,383 |
| jibe | 9,214 |
| jobvite | 7,540 |
| feishujobs | 7,237 |
| jobappnetwork | 6,267 |
| betterteam | 6,252 |
| avature | 6,178 |
| cornerstone | 5,960 |
| comeet | 5,899 |
| trakstar | 5,883 |
| gr8people | 4,015 |
| arbeitsagentur | 3,711 |
| hibob | 3,456 |
| solides | 3,456 |
| catsone | 3,282 |
| epam | 3,168 |
| gem | 3,061 |
| freshteam | 3,060 |
| workstream | 3,040 |
| factorial | 2,869 |
| pageup | 2,844 |
| inhire | 2,260 |
| hrmos | 2,138 |
| herp | 1,871 |
| traffit | 1,809 |
| earcu | 1,789 |
| potok | 1,525 |
| gusto | 1,524 |
| peopleforce | 1,144 |
| softgarden | 1,144 |
| eploy | 1,068 |
| hrmanager | 984 |
| humanbit | 913 |
| jobscore | 870 |
| neogov | 848 |
| talentlyft | 709 |
| loxo | 702 |
| deel | 696 |
| keka | 622 |
| luxoft | 575 |
| senior | 555 |
| peoplesoft | 533 |
| emply | 442 |
| beesite | 430 |
| huntflow | 369 |
| cleverstaff | 287 |
| wpyoast | 243 |
| hiringroom | 205 |
| applitrack | 198 |
| friendwork | 185 |
| erecruiter | 165 |
| mokahr | 137 |
| ashbygraphql | 123 |
| opencats | 97 |
| dover | 93 |
| workablemarketplace | 92 |
| scalis | 83 |
| geekhunter | 57 |
| ismartrecruit | 54 |
| bullhorn | 48 |
| compleo | 44 |
| quickin | 43 |
| careerspage | 40 |
| vention | 40 |
| northstone | 23 |
| hurma | 22 |
| rapyd | 21 |
| recrutei | 20 |
| speedrun | 15 |
| pyjamahr | 13 |
| likeit | 12 |
| odoo | 10 |
| recruiterflow | 10 |
| clinch | 8 |
| enlizt | 2 |
| spark | 1 |
| talentadore | 1 |
| talenthr | 1 |

## Aggregators & job boards

**133 with postings · 444,654 open postings.**

| Source | Open jobs |
| --- | ---: |
| mycareersfuture | 53,438 |
| energyjobline | 52,965 |
| adzuna | 45,178 |
| eures | 34,487 |
| jobstreet | 16,014 |
| whatjobs-uk | 14,404 |
| startupjobs | 12,226 |
| 4dayweek | 10,390 |
| telegram | 9,164 |
| whatjobs-es | 8,548 |
| seek | 8,067 |
| himalayas | 7,601 |
| jobstash | 7,327 |
| gulftalent | 7,324 |
| whatjobs-sg | 6,784 |
| whatjobs-pl | 6,719 |
| whatjobs-it | 6,639 |
| whatjobs-ca | 6,537 |
| whatjobs-nl | 6,516 |
| whatjobs-fr | 6,231 |
| djinni | 6,202 |
| reed | 6,078 |
| usajobs | 5,916 |
| jobleads | 5,643 |
| hh | 4,771 |
| arbeitnow | 4,584 |
| whatjobs-ph | 4,538 |
| whatjobs-za | 3,698 |
| workatastartup | 3,580 |
| whatjobs | 3,396 |
| whatjobs-au | 3,223 |
| powertofly | 3,079 |
| jobnet | 2,715 |
| wttj | 2,489 |
| whatjobs-id | 2,481 |
| jobtech | 2,474 |
| whatjobs-mx | 2,204 |
| nofluffjobs | 2,002 |
| whatjobs-ae | 1,971 |
| whatjobs-ch | 1,957 |
| whatjobs-my | 1,830 |
| whatjobs-in | 1,745 |
| infojobs | 1,691 |
| whatjobs-br | 1,664 |
| whatjobs-ie | 1,635 |
| jobylon | 1,612 |
| aijobs | 1,528 |
| wellfound | 1,524 |
| whatjobs-pk | 1,399 |
| whatjobs-ar | 1,354 |
| whatjobs-co | 1,342 |
| work180 | 1,329 |
| torre | 1,309 |
| whatjobs-de | 1,283 |
| whatjobs-hk | 1,102 |
| whatjobs-pt | 1,072 |
| getonbrd | 1,070 |
| thehub | 976 |
| remotedotcom | 957 |
| tyomarkkinatori | 886 |
| profession | 884 |
| whatjobs-cl | 867 |
| instaffo | 850 |
| getmatch | 843 |
| jobboardly | 824 |
| whatjobs-tr | 789 |
| whatjobs-dk | 733 |
| whatjobs-sa | 702 |
| whatjobs-uy | 641 |
| itviec | 624 |
| whatjobs-be | 580 |
| whatjobs-fi | 578 |
| whatjobs-nz | 571 |
| jobdanmark | 557 |
| habr_career | 492 |
| solidjobs | 429 |
| themuse | 398 |
| whatjobs-qa | 391 |
| whatjobs-pe | 378 |
| kalibrr | 376 |
| whatjobs-se | 374 |
| wuzzuf | 359 |
| weworkremotely | 342 |
| whatjobs-no | 318 |
| remoteok | 314 |
| schoolspring | 308 |
| herkey | 206 |
| whatjobs-pr | 201 |
| techinasia | 196 |
| geekjob | 187 |
| gradcracker | 167 |
| wantapply | 163 |
| whatjobs-kw | 154 |
| crelate | 153 |
| japandev | 146 |
| whatjobs-om | 134 |
| remotli | 132 |
| hackernews | 122 |
| vagas | 122 |
| getro | 93 |
| whatjobs-lu | 92 |
| whatjobs-bh | 82 |
| techtree | 81 |
| whatjobs-ve | 78 |
| tecla | 62 |
| whatjobs-th | 61 |
| getmanfred | 57 |
| cryptocurrencyjobs | 55 |
| whatjobs-at | 52 |
| whatjobs-ng | 45 |
| landingjobs | 43 |
| joppy | 42 |
| whatjobs-pa | 40 |
| whatjobs-cr | 38 |
| edjoin | 35 |
| nodesk | 35 |
| startupandvc | 27 |
| workingnomads | 27 |
| remotive | 25 |
| pythonorg | 19 |
| whatjobs-ma | 15 |
| whatjobs-gt | 14 |
| golangjobs | 12 |
| whatjobs-dz | 12 |
| topco | 10 |
| jobspresso | 7 |
| jobicy | 6 |
| whatjobs-ug | 5 |
| whatjobs-ec | 4 |
| whatjobs-tz | 4 |
| teamex | 1 |
| whatjobs-eg | 1 |
| whatjobs-ru | 1 |

## Company career sites

**52 feeds with postings · 23,293 open postings.**

| Source | Open jobs |
| --- | ---: |
| amazon | 5,378 |
| apple | 3,101 |
| google | 2,798 |
| alibaba | 1,542 |
| sber | 1,407 |
| alfabank | 1,167 |
| ibm | 937 |
| mts | 782 |
| meta | 765 |
| emagine | 738 |
| tencent | 676 |
| meituan | 672 |
| yandex | 345 |
| tbank | 264 |
| uber | 264 |
| dassault | 244 |
| vk | 242 |
| x5 | 235 |
| megafon | 182 |
| rostelecom | 165 |
| lamoda | 130 |
| micro1 | 128 |
| orion | 126 |
| avito | 97 |
| bairesdev | 92 |
| dataart | 92 |
| kaspersky | 79 |
| uzum | 70 |
| halyk | 59 |
| garena | 52 |
| rwb | 49 |
| staffy | 45 |
| yandexcrowd | 45 |
| magnit | 42 |
| kaspi | 40 |
| kontur | 27 |
| tochka | 26 |
| dodo | 24 |
| selectel | 24 |
| aviasales | 21 |
| lesta | 19 |
| ozon | 18 |
| ptsecurity | 17 |
| domclick | 16 |
| onstrider | 13 |
| 2gis | 12 |
| lumenalta | 11 |
| telegramcareers | 5 |
| mtslink | 4 |
| alignerr | 3 |
| jettycloud | 2 |
| kuper | 1 |

Plus **46** postings from manually imported links and bulk imports.
